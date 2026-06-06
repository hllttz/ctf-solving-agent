package sandbox

import (
	"reflect"
	"testing"
)

func TestDockerWriteFileArgsCreatesParentDirectory(t *testing.T) {
	got := dockerWriteFileArgs("ctf-test", "/workspace/scripts/solve.py")
	want := []string{
		"docker", "exec", "-i", "ctf-test", "bash", "-c",
		"mkdir -p '/workspace/scripts' && tee '/workspace/scripts/solve.py' > /dev/null",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerWriteFileArgs = %#v, want %#v", got, want)
	}
}

func TestDockerWriteFileArgsSkipsRootParent(t *testing.T) {
	got := dockerWriteFileArgs("ctf-test", "/solve.py")
	want := []string{
		"docker", "exec", "-i", "ctf-test", "bash", "-c",
		"tee '/solve.py' > /dev/null",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerWriteFileArgs = %#v, want %#v", got, want)
	}
}

func TestDockerWriteFileArgsEscapesQuotes(t *testing.T) {
	got := dockerWriteFileArgs("ctf-test", "/workspace/a'b/solve.py")
	want := []string{
		"docker", "exec", "-i", "ctf-test", "bash", "-c",
		"mkdir -p '/workspace/a'\\''b' && tee '/workspace/a'\\''b/solve.py' > /dev/null",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerWriteFileArgs = %#v, want %#v", got, want)
	}
}
