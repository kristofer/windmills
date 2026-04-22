package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestPhase7InitProvidesUserlandPrograms(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase6ResetForTest()
	phase2Init()
	if !phase7Init() {
		t.Fatalf("phase7Init should succeed")
	}
	for _, path := range []string{
		"/init",
		"/bin/sh",
		"/bin/ls",
		"/bin/cat",
		"/bin/echo",
		"/bin/kill",
		"/bin/ps",
	} {
		node, ok := fsLookupByPath(nil, path)
		if !ok || node.kind != inodeKindFile {
			t.Fatalf("%s should be present as a file", path)
		}
	}
}

func TestPhase7ShellPipelineAndRedirection(t *testing.T) {
	p := setupPhase6TestProcess(t)
	if !phase7Init() {
		t.Fatalf("phase7Init should succeed")
	}
	if _, ok := fsLookupByPath(nil, "/tmp"); !ok {
		if _, ok := fsCreatePath(nil, "/tmp", inodeKindDir, deviceNone); !ok {
			t.Fatalf("create /tmp should succeed")
		}
	}
	out, ok := phase7RunShellLine(p, "echo hello world | cat > /tmp/msg", nil)
	if !ok {
		t.Fatalf("pipeline with redirection should succeed")
	}
	if out != "" {
		t.Fatalf("pipeline output = %q, want empty", out)
	}
	out, ok = phase7RunShellLine(p, "cat < /tmp/msg", nil)
	if !ok {
		t.Fatalf("cat redirected input should succeed")
	}
	if out != "hello world\n" {
		t.Fatalf("cat output = %q, want %q", out, "hello world\n")
	}
	out, ok = phase7RunShellLine(p, "ls /bin", nil)
	if !ok {
		t.Fatalf("ls should succeed")
	}
	if !strings.Contains(out, "sh\n") {
		t.Fatalf("ls /bin output should include sh entry, got %q", out)
	}
}

func TestPhase7KillAndPsUtilities(t *testing.T) {
	p := setupPhase6TestProcess(t)
	if !phase7Init() {
		t.Fatalf("phase7Init should succeed")
	}
	worker, ok := allocProcess("worker", func() {}, p.pid)
	if !ok {
		t.Fatalf("allocProcess worker should succeed")
	}
	out, ok := phase7RunShellLine(p, "kill "+strconv.Itoa(worker.pid), nil)
	if !ok {
		t.Fatalf("kill utility should succeed")
	}
	if out != "" {
		t.Fatalf("kill output = %q, want empty", out)
	}
	if worker.state != procZombie {
		t.Fatalf("killed process state = %v, want %v", worker.state, procZombie)
	}
	out, ok = phase7RunShellLine(p, "ps", nil)
	if !ok {
		t.Fatalf("ps should succeed")
	}
	if !strings.Contains(out, strconv.Itoa(worker.pid)+" ZOMBIE worker") {
		t.Fatalf("ps output %q should include worker zombie line", out)
	}
}
