package scanner

import (
	"os"
	"path/filepath"
	"sort"
)

// Node is one entry of the disk map: a file or a folder with its total size.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path,omitempty"`
	IsDir    bool    `json:"is_dir"`
	Size     int64   `json:"size"`
	Children []*Node `json:"children,omitempty"`
}

// maxChildren caps how many entries a folder lists; the rest are merged.
const maxChildren = 40

// BuildTree sizes root down to depth levels below it. Folders deeper than that
// are sized in full but not expanded. onTop, if set, reports progress over the
// top-level entries of root.
func BuildTree(root string, depth int, onTop func(done, total int)) (*Node, []string) {
	root = filepath.Clean(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []string{err.Error()}
	}

	node := &Node{Name: filepath.Base(root), Path: root, IsDir: true}
	var errs []string
	for i, e := range entries {
		child, childErrs := buildNode(filepath.Join(root, e.Name()), depth-1)
		errs = append(errs, childErrs...)
		if child != nil {
			node.Children = append(node.Children, child)
			node.Size += child.Size
		}
		if onTop != nil {
			onTop(i+1, len(entries))
		}
	}
	node.Children = prune(node.Children, node.Size)
	return node, errs
}

func buildNode(p string, depth int) (*Node, []string) {
	info, err := os.Lstat(p)
	if err != nil {
		return nil, []string{err.Error()}
	}
	// Symlinks are not followed, so they never add size or loops.
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	name := filepath.Base(p)
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, nil
		}
		return &Node{Name: name, Path: p, Size: diskSize(info)}, nil
	}

	if depth <= 0 {
		size, errs := SizeOf(p)
		return &Node{Name: name, Path: p, IsDir: true, Size: size}, errs
	}

	entries, err := os.ReadDir(p)
	if err != nil {
		return &Node{Name: name, Path: p, IsDir: true}, []string{err.Error()}
	}
	node := &Node{Name: name, Path: p, IsDir: true}
	var errs []string
	for _, e := range entries {
		child, childErrs := buildNode(filepath.Join(p, e.Name()), depth-1)
		errs = append(errs, childErrs...)
		if child != nil {
			node.Children = append(node.Children, child)
			node.Size += child.Size
		}
	}
	node.Children = prune(node.Children, node.Size)
	return node, errs
}

// prune keeps the largest children and merges the rest into one "outros"
// entry, so the map stays readable. Anything under 0.2% of the parent is merged.
func prune(children []*Node, total int64) []*Node {
	sort.SliceStable(children, func(i, j int) bool { return children[i].Size > children[j].Size })
	var keep []*Node
	var other int64
	for i, c := range children {
		big := total == 0 || c.Size*500 >= total
		if i < maxChildren && big {
			keep = append(keep, c)
		} else {
			other += c.Size
		}
	}
	if other > 0 {
		keep = append(keep, &Node{Name: "outros", Size: other})
	}
	return keep
}
