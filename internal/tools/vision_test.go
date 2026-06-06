package tools

import (
	"reflect"
	"testing"
)

func TestCandidateImagePathsPreservesRelativeSubdirectories(t *testing.T) {
	got := candidateImagePaths("images/qr.png")
	want := []string{
		"images/qr.png",
		"/challenge/distfiles/images/qr.png",
		"/workspace/images/qr.png",
		"/challenge/workspace/images/qr.png",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidateImagePaths = %#v, want %#v", got, want)
	}
}

func TestCandidateImagePathsKeepsAbsolutePathOnly(t *testing.T) {
	got := candidateImagePaths("/tmp/qr.png")
	want := []string{"/tmp/qr.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidateImagePaths = %#v, want %#v", got, want)
	}
}

func TestCandidateImagePathsDoesNotExpandEscapingRelativePath(t *testing.T) {
	got := candidateImagePaths("../secret.png")
	want := []string{"../secret.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidateImagePaths = %#v, want %#v", got, want)
	}
}
