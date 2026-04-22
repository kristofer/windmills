package main

import (
	"slices"
	"strconv"
	"strings"
)

const (
	phase7InitPath = "/init"
	phase7BinDir   = "/bin"
)

func phase7Init() bool {
	if !phase6Init() {
		return false
	}
	if !phase7EnsureDir(phase7BinDir) {
		return false
	}
	if !phase7EnsureFile(phase7InitPath, []byte("sh\n")) {
		return false
	}
	for _, path := range []string{
		"/bin/sh",
		"/bin/ls",
		"/bin/cat",
		"/bin/echo",
		"/bin/kill",
		"/bin/ps",
	} {
		if !phase7EnsureFile(path, []byte(path+"\n")) {
			return false
		}
	}
	return true
}

func phase7EnsureDir(path string) bool {
	node, ok := fsLookupByPath(nil, path)
	if ok {
		return node.kind == inodeKindDir
	}
	_, ok = fsCreatePath(nil, path, inodeKindDir, deviceNone)
	return ok
}

func phase7EnsureFile(path string, data []byte) bool {
	node, ok := fsLookupByPath(nil, path)
	if !ok {
		node, ok = fsCreatePath(nil, path, inodeKindFile, deviceNone)
	}
	if !ok || node.kind != inodeKindFile {
		return false
	}
	node.size = 0
	wrote := fsWriteRegular(node, 0, data)
	return wrote == len(data)
}

func phase7ReadFile(p *process, path string) ([]byte, bool) {
	node, ok := fsLookupByPath(p, path)
	if !ok || node.kind != inodeKindFile {
		return nil, false
	}
	data := make([]byte, node.size)
	copy(data, node.data[:node.size])
	return data, true
}

func phase7WriteFile(p *process, path string, data []byte) bool {
	node, ok := fsLookupByPath(p, path)
	if !ok {
		node, ok = fsCreatePath(p, path, inodeKindFile, deviceNone)
	}
	if !ok || node.kind != inodeKindFile {
		return false
	}
	node.size = 0
	wrote := fsWriteRegular(node, 0, data)
	return wrote == len(data)
}

func phase7RunInit(p *process, commandLine string) (string, bool) {
	if !phase7Init() {
		return "", false
	}
	line := strings.TrimSpace(commandLine)
	if line == "" {
		payload, ok := phase7ReadFile(p, phase7InitPath)
		if !ok {
			return "", false
		}
		line = strings.TrimSpace(string(payload))
	}
	if line == "" {
		return "", true
	}
	return phase7RunShellLine(p, line, nil)
}

func phase7RunShellLine(p *process, line string, stdin []byte) (string, bool) {
	segments := strings.Split(line, "|")
	in := append([]byte(nil), stdin...)
	for i, rawSegment := range segments {
		cmd, args, inputPath, outputPath, ok := phase7ParseCommandSegment(rawSegment)
		if !ok {
			return "", false
		}
		if inputPath != "" {
			in, ok = phase7ReadFile(p, inputPath)
			if !ok {
				return "", false
			}
		}
		out, ok := phase7RunCommand(p, cmd, args, in)
		if !ok {
			return "", false
		}
		if i < len(segments)-1 && outputPath != "" {
			return "", false
		}
		if outputPath != "" {
			if !phase7WriteFile(p, outputPath, out) {
				return "", false
			}
			out = nil
		}
		in = out
	}
	return string(in), true
}

func phase7ParseCommandSegment(segment string) (cmd string, args []string, inputPath, outputPath string, ok bool) {
	tokens := strings.Fields(strings.TrimSpace(segment))
	if len(tokens) == 0 {
		return "", nil, "", "", false
	}
	words := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "<":
			if i+1 >= len(tokens) || inputPath != "" {
				return "", nil, "", "", false
			}
			inputPath = tokens[i+1]
			i++
		case ">":
			if i+1 >= len(tokens) || outputPath != "" {
				return "", nil, "", "", false
			}
			outputPath = tokens[i+1]
			i++
		default:
			words = append(words, tokens[i])
		}
	}
	if len(words) == 0 {
		return "", nil, "", "", false
	}
	return words[0], words[1:], inputPath, outputPath, true
}

func phase7RunCommand(p *process, cmd string, args []string, stdin []byte) ([]byte, bool) {
	switch cmd {
	case "echo":
		return []byte(strings.Join(args, " ") + "\n"), true
	case "cat":
		if len(args) == 0 {
			return append([]byte(nil), stdin...), true
		}
		var out []byte
		for _, path := range args {
			data, ok := phase7ReadFile(p, path)
			if !ok {
				return nil, false
			}
			out = append(out, data...)
		}
		return out, true
	case "ls":
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		node, ok := fsLookupByPath(p, path)
		if !ok || node.kind != inodeKindDir {
			return nil, false
		}
		names := make([]string, 0, len(node.dir))
		for i := range node.dir {
			if node.dir[i].used {
				names = append(names, node.dir[i].name)
			}
		}
		slices.Sort(names)
		if len(names) == 0 {
			return nil, true
		}
		return []byte(strings.Join(names, "\n") + "\n"), true
	case "kill":
		if len(args) != 1 {
			return nil, false
		}
		pid, err := strconv.Atoi(args[0])
		if err != nil || pid < 0 {
			return nil, false
		}
		if sysKill(p, pid) == syscallError {
			return nil, false
		}
		return nil, true
	case "ps":
		var lines []string
		for i := range processTable {
			proc := processTable[i]
			if proc.state == procUnused {
				continue
			}
			lines = append(lines, strconv.Itoa(proc.pid)+" "+phase7ProcStateName(proc.state)+" "+proc.name)
		}
		slices.Sort(lines)
		if len(lines) == 0 {
			return nil, true
		}
		return []byte(strings.Join(lines, "\n") + "\n"), true
	case "sh":
		if len(args) == 0 {
			return nil, true
		}
		script, ok := phase7ReadFile(p, args[0])
		if !ok {
			return nil, false
		}
		lines := strings.Split(string(script), "\n")
		var out []byte
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			chunk, ok := phase7RunShellLine(p, line, nil)
			if !ok {
				return nil, false
			}
			out = append(out, []byte(chunk)...)
		}
		return out, true
	default:
		return nil, false
	}
}

func phase7ProcStateName(state procState) string {
	switch state {
	case procEmbryo:
		return "EMBRYO"
	case procRunnable:
		return "RUNNABLE"
	case procRunning:
		return "RUNNING"
	case procSleeping:
		return "SLEEPING"
	case procZombie:
		return "ZOMBIE"
	default:
		return "UNUSED"
	}
}
