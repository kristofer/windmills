package main

import (
	"strconv"
	"strings"
)

const (
	maxOpenFiles      = 16
	maxPathLength     = 128
	maxSyscallIOBytes = 4096

	phase6MaxInodes     = 64
	phase6MaxDirEntries = 32
	phase6MaxFileBytes  = 4096
)

const (
	openRead uintptr = 1 << iota
	openWrite
	openCreate
)

type inodeKind uint8

const (
	inodeKindNone inodeKind = iota
	inodeKindFile
	inodeKindDir
	inodeKindDevice
)

type deviceKind uint8

const (
	deviceNone deviceKind = iota
	deviceConsole
	deviceUART
	deviceTimer
)

type dirEntry struct {
	used    bool
	name    string
	inodeID int
}

type inode struct {
	used   bool
	id     int
	parent int
	nlink  int
	kind   inodeKind
	device deviceKind
	size   int
	data   [phase6MaxFileBytes]byte
	dir    [phase6MaxDirEntries]dirEntry
}

type fileDescriptor struct {
	used     bool
	inodeID  int
	offset   int
	readable bool
	writable bool
}

type fileSystem struct {
	initialized bool
	nextInodeID int
	inodes      [phase6MaxInodes]inode
}

var (
	globalFS          fileSystem
	consoleDeviceSink []byte
	uartDeviceSink    []byte
)

func phase6ResetForTest() {
	globalFS = fileSystem{}
	consoleDeviceSink = nil
	uartDeviceSink = nil
}

func phase6Init() bool {
	if globalFS.initialized {
		return true
	}
	globalFS.nextInodeID = 1
	root, ok := fsAllocInode(inodeKindDir, 0, deviceNone)
	if !ok {
		return false
	}
	root.parent = root.id
	root.nlink = 1
	globalFS.initialized = true
	if _, ok := fsCreatePath(nil, "/dev", inodeKindDir, deviceNone); !ok {
		globalFS.initialized = false
		return false
	}
	if _, ok := fsCreatePath(nil, "/dev/console", inodeKindDevice, deviceConsole); !ok {
		globalFS.initialized = false
		return false
	}
	if _, ok := fsCreatePath(nil, "/dev/uart", inodeKindDevice, deviceUART); !ok {
		globalFS.initialized = false
		return false
	}
	if _, ok := fsCreatePath(nil, "/dev/timer", inodeKindDevice, deviceTimer); !ok {
		globalFS.initialized = false
		return false
	}
	initInode, ok := fsCreatePath(nil, "/init", inodeKindFile, deviceNone)
	if !ok {
		globalFS.initialized = false
		return false
	}
	_ = fsWriteRegular(initInode, 0, []byte("init\n"))
	return true
}

func fsAllocInode(kind inodeKind, parent int, device deviceKind) (*inode, bool) {
	for i := range globalFS.inodes {
		if globalFS.inodes[i].used {
			continue
		}
		globalFS.inodes[i] = inode{
			used:   true,
			id:     globalFS.nextInodeID,
			parent: parent,
			nlink:  1,
			kind:   kind,
			device: device,
		}
		globalFS.nextInodeID++
		return &globalFS.inodes[i], true
	}
	return nil, false
}

func fsLookupByID(id int) (*inode, bool) {
	if id <= 0 {
		return nil, false
	}
	for i := range globalFS.inodes {
		if !globalFS.inodes[i].used || globalFS.inodes[i].id != id {
			continue
		}
		return &globalFS.inodes[i], true
	}
	return nil, false
}

func fsRootInodeID() int {
	root, ok := fsLookupByID(1)
	if !ok {
		return 0
	}
	return root.id
}

func fsLookupInDir(dir *inode, name string) (*inode, bool) {
	if dir == nil || dir.kind != inodeKindDir {
		return nil, false
	}
	for i := range dir.dir {
		entry := dir.dir[i]
		if !entry.used || entry.name != name {
			continue
		}
		return fsLookupByID(entry.inodeID)
	}
	return nil, false
}

func fsAddDirEntry(dir *inode, name string, childID int) bool {
	if dir == nil || dir.kind != inodeKindDir || name == "" || strings.Contains(name, "/") {
		return false
	}
	if _, exists := fsLookupInDir(dir, name); exists {
		return false
	}
	for i := range dir.dir {
		if dir.dir[i].used {
			continue
		}
		dir.dir[i] = dirEntry{used: true, name: name, inodeID: childID}
		return true
	}
	return false
}

func fsRemoveDirEntry(dir *inode, name string) (int, bool) {
	if dir == nil || dir.kind != inodeKindDir {
		return 0, false
	}
	for i := range dir.dir {
		entry := dir.dir[i]
		if !entry.used || entry.name != name {
			continue
		}
		dir.dir[i] = dirEntry{}
		return entry.inodeID, true
	}
	return 0, false
}

func fsSplitPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

func fsStartingDir(p *process, path string) (*inode, bool) {
	root, ok := fsLookupByID(1)
	if !ok {
		return nil, false
	}
	if strings.HasPrefix(path, "/") || p == nil {
		return root, true
	}
	if p.cwdInode == 0 {
		return root, true
	}
	return fsLookupByID(p.cwdInode)
}

func fsLookupByPath(p *process, path string) (*inode, bool) {
	if !phase6Init() {
		return nil, false
	}
	if path == "" {
		return nil, false
	}
	start, ok := fsStartingDir(p, path)
	if !ok {
		return nil, false
	}
	parts := fsSplitPath(path)
	current := start
	if len(parts) == 0 {
		return current, true
	}
	for _, part := range parts {
		if part == ".." {
			next, ok := fsLookupByID(current.parent)
			if !ok {
				return nil, false
			}
			current = next
			continue
		}
		next, ok := fsLookupInDir(current, part)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func fsResolveParent(p *process, path string) (*inode, string, bool) {
	if path == "" || path == "/" {
		return nil, "", false
	}
	parts := fsSplitPath(path)
	if len(parts) == 0 {
		return nil, "", false
	}
	name := parts[len(parts)-1]
	parentPath := "."
	if strings.HasPrefix(path, "/") {
		parentPath = "/"
	}
	if len(parts) > 1 {
		if strings.HasPrefix(path, "/") {
			parentPath += strings.Join(parts[:len(parts)-1], "/")
		} else {
			parentPath = strings.Join(parts[:len(parts)-1], "/")
		}
	}
	parent, ok := fsLookupByPath(p, parentPath)
	if !ok || parent.kind != inodeKindDir {
		return nil, "", false
	}
	return parent, name, true
}

func fsCreatePath(p *process, path string, kind inodeKind, device deviceKind) (*inode, bool) {
	parent, name, ok := fsResolveParent(p, path)
	if !ok {
		return nil, false
	}
	if _, exists := fsLookupInDir(parent, name); exists {
		return nil, false
	}
	node, ok := fsAllocInode(kind, parent.id, device)
	if !ok {
		return nil, false
	}
	if kind == inodeKindDir {
		node.parent = parent.id
	}
	if !fsAddDirEntry(parent, name, node.id) {
		*node = inode{}
		return nil, false
	}
	return node, true
}

func fsWriteRegular(node *inode, offset int, src []byte) int {
	if node == nil || node.kind != inodeKindFile || offset < 0 || offset >= phase6MaxFileBytes {
		return 0
	}
	remaining := phase6MaxFileBytes - offset
	if len(src) < remaining {
		remaining = len(src)
	}
	if remaining <= 0 {
		return 0
	}
	copy(node.data[offset:offset+remaining], src[:remaining])
	if end := offset + remaining; end > node.size {
		node.size = end
	}
	return remaining
}

func fsReadRegular(node *inode, offset int, dst []byte) int {
	if node == nil || node.kind != inodeKindFile || offset < 0 || offset >= node.size {
		return 0
	}
	available := node.size - offset
	if len(dst) < available {
		available = len(dst)
	}
	copy(dst[:available], node.data[offset:offset+available])
	return available
}

func allocFD(p *process, inodeID int, readable, writable bool) (int, bool) {
	if p == nil {
		return 0, false
	}
	for fd := range p.fdTable {
		if p.fdTable[fd].used {
			continue
		}
		p.fdTable[fd] = fileDescriptor{
			used:     true,
			inodeID:  inodeID,
			readable: readable,
			writable: writable,
		}
		return fd, true
	}
	return 0, false
}

func lookupFD(p *process, fd int) (*fileDescriptor, bool) {
	if p == nil || fd < 0 || fd >= len(p.fdTable) {
		return nil, false
	}
	if !p.fdTable[fd].used {
		return nil, false
	}
	return &p.fdTable[fd], true
}

func sysOpen(p *process, path string, flags uintptr) uintptr {
	if p == nil || path == "" || !phase6Init() {
		return syscallError
	}
	readable := flags&openRead != 0
	writable := flags&openWrite != 0
	if !readable && !writable {
		readable = true
	}
	node, ok := fsLookupByPath(p, path)
	if !ok && flags&openCreate != 0 {
		node, ok = fsCreatePath(p, path, inodeKindFile, deviceNone)
	}
	if !ok || node.kind == inodeKindDir && writable {
		return syscallError
	}
	fd, ok := allocFD(p, node.id, readable, writable)
	if !ok {
		return syscallError
	}
	return uintptr(fd)
}

func sysClose(p *process, fd int) uintptr {
	entry, ok := lookupFD(p, fd)
	if !ok {
		return syscallError
	}
	*entry = fileDescriptor{}
	return 0
}

func sysRead(p *process, fd int, dstVA, count uintptr) uintptr {
	entry, ok := lookupFD(p, fd)
	if !ok || !entry.readable || count > maxSyscallIOBytes {
		return syscallError
	}
	node, ok := fsLookupByID(entry.inodeID)
	if !ok {
		return syscallError
	}
	n := int(count)
	buf := make([]byte, n)
	switch node.kind {
	case inodeKindFile:
		n = fsReadRegular(node, entry.offset, buf)
	case inodeKindDevice:
		n = fsReadDevice(node.device, entry.offset, buf)
	default:
		return syscallError
	}
	if n == 0 {
		return 0
	}
	if !copyout(p, dstVA, buf[:n]) {
		return syscallError
	}
	entry.offset += n
	return uintptr(n)
}

func sysWrite(p *process, fd int, srcVA, count uintptr) uintptr {
	entry, ok := lookupFD(p, fd)
	if !ok || !entry.writable || count > maxSyscallIOBytes {
		return syscallError
	}
	node, ok := fsLookupByID(entry.inodeID)
	if !ok {
		return syscallError
	}
	n := int(count)
	buf := make([]byte, n)
	if !copyin(p, buf, srcVA) {
		return syscallError
	}
	switch node.kind {
	case inodeKindFile:
		n = fsWriteRegular(node, entry.offset, buf)
	case inodeKindDevice:
		n = fsWriteDevice(node.device, buf)
	default:
		return syscallError
	}
	entry.offset += n
	return uintptr(n)
}

func sysMkdir(p *process, path string) uintptr {
	if !phase6Init() {
		return syscallError
	}
	if _, ok := fsCreatePath(p, path, inodeKindDir, deviceNone); !ok {
		return syscallError
	}
	return 0
}

func sysChdir(p *process, path string) uintptr {
	if p == nil {
		return syscallError
	}
	node, ok := fsLookupByPath(p, path)
	if !ok || node.kind != inodeKindDir {
		return syscallError
	}
	p.cwdInode = node.id
	return 0
}

func sysLink(p *process, oldPath, newPath string) uintptr {
	if !phase6Init() {
		return syscallError
	}
	oldNode, ok := fsLookupByPath(p, oldPath)
	if !ok || oldNode.kind == inodeKindDir {
		return syscallError
	}
	parent, name, ok := fsResolveParent(p, newPath)
	if !ok {
		return syscallError
	}
	if !fsAddDirEntry(parent, name, oldNode.id) {
		return syscallError
	}
	oldNode.nlink++
	return 0
}

func fsDirEmpty(node *inode) bool {
	if node == nil || node.kind != inodeKindDir {
		return false
	}
	for i := range node.dir {
		if node.dir[i].used {
			return false
		}
	}
	return true
}

func sysUnlink(p *process, path string) uintptr {
	if !phase6Init() {
		return syscallError
	}
	parent, name, ok := fsResolveParent(p, path)
	if !ok {
		return syscallError
	}
	removedID, ok := fsRemoveDirEntry(parent, name)
	if !ok {
		return syscallError
	}
	node, ok := fsLookupByID(removedID)
	if !ok {
		return syscallError
	}
	if node.kind == inodeKindDir && !fsDirEmpty(node) {
		_ = fsAddDirEntry(parent, name, removedID)
		return syscallError
	}
	node.nlink--
	if node.nlink <= 0 {
		*node = inode{}
	}
	return 0
}

func fsWriteDevice(device deviceKind, src []byte) int {
	switch device {
	case deviceConsole:
		consoleDeviceSink = append(consoleDeviceSink, src...)
		return len(src)
	case deviceUART:
		uartDeviceSink = append(uartDeviceSink, src...)
		return len(src)
	default:
		return 0
	}
}

func fsReadDevice(device deviceKind, offset int, dst []byte) int {
	if device != deviceTimer {
		return 0
	}
	payload := []byte(strconv.FormatUint(monotonicTick(), 10))
	if offset >= len(payload) {
		return 0
	}
	n := len(payload) - offset
	if len(dst) < n {
		n = len(dst)
	}
	copy(dst[:n], payload[offset:offset+n])
	return n
}

func copyinstr(p *process, srcVA uintptr, maxLen int) (string, bool) {
	if p == nil || maxLen <= 0 {
		return "", false
	}
	buf := make([]byte, 1)
	out := make([]byte, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		if !copyin(p, buf, srcVA+uintptr(i)) {
			return "", false
		}
		if buf[0] == 0 {
			return string(out), true
		}
		out = append(out, buf[0])
	}
	return "", false
}
