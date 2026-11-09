package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildTreeSizesAddUp(t *testing.T) {
	root := t.TempDir()
	a1 := writeFile(t, root, "a/one.bin", 1000)
	a2 := writeFile(t, root, "a/deep/two.bin", 500)
	b := writeFile(t, root, "b/three.bin", 250)
	top := writeFile(t, root, "top.bin", 100)

	tree, errs := BuildTree(root, 2, nil)
	if tree == nil {
		t.Fatalf("no tree, errs=%v", errs)
	}
	if want := a1 + a2 + b + top; tree.Size != want {
		t.Errorf("root size = %d, want %d", tree.Size, want)
	}

	var sum int64
	for _, c := range tree.Children {
		sum += c.Size
	}
	if sum != tree.Size {
		t.Errorf("children sum = %d, root = %d", sum, tree.Size)
	}
}

func TestBuildTreeDepthLimitsExpansion(t *testing.T) {
	root := t.TempDir()
	want := writeFile(t, root, "a/b/c/file.bin", 64)

	tree, _ := BuildTree(root, 1, nil)
	var a *Node
	for _, c := range tree.Children {
		if c.Name == "a" {
			a = c
		}
	}
	if a == nil {
		t.Fatal("folder a missing")
	}
	// depth 1 expands only one level below the root, so "a" has no children listed.
	if len(a.Children) != 0 {
		t.Errorf("a should not be expanded at depth 1, got %d children", len(a.Children))
	}
	if a.Size != want {
		t.Errorf("a size = %d, want %d (still sized in full)", a.Size, want)
	}
}

func TestBuildTreeSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "big.bin", 9000)
	want := writeFile(t, root, "real.bin", 10)
	if err := os.Symlink(filepath.Join(outside, "big.bin"), filepath.Join(root, "link.bin")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	tree, _ := BuildTree(root, 2, nil)
	if tree.Size != want {
		t.Errorf("size = %d, want %d (symlink target must not count)", tree.Size, want)
	}
}

func TestPruneMergesSmallEntries(t *testing.T) {
	var children []*Node
	for i := 0; i < 5; i++ {
		children = append(children, &Node{Name: "big", Size: 1000})
	}
	children = append(children, &Node{Name: "tiny", Size: 1})

	got := prune(children, 5001)
	last := got[len(got)-1]
	if last.Name != "outros" || last.Size != 1 {
		t.Errorf("tiny entry should merge into outros, last = %+v", last)
	}
}
