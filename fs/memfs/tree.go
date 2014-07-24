package memfs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"unicode"
)

// Box drawings symbols - http://unicode-table.com/en/sections/box-drawing/.
var (
	boxVerticalRight = []byte("├")
	boxHorizontal    = []byte("─")
	boxVertical      = []byte("│")
	boxUpRight       = []byte("└")
	boxSpace         = []byte{'\u0020'}
	boxHardSpace     = []byte{'\u00A0'}
)

func max(i, j int) int {
	if i > j {
		return i
	}
	return j
}

// CustomTree instructs tree builder how to parse single line of given buffer,
// where 'name' is the name of a tree node, 'depth' is its depth in the tree
// and 'err' eventual parsing failure. The 'line' is guaranteed to be non-nil
// non-empty.
type CustomTree func(line []byte) (depth int, name []byte, err error)

// Unix is a tree builder for the 'tree' Unix command.
var Unix CustomTree

// Tab is a tree builder for simplified tree representation, where each level
// is idented with one tabulation character (\t) only.
var Tab CustomTree

func init() {
	Unix = func(p []byte) (depth int, name []byte, err error) {
		var n int
		// TODO(rjeczalik): Count up to first non-box character.
		depth = (bytes.Count(p, boxSpace) + bytes.Count(p, boxHardSpace) +
			bytes.Count(p, boxVertical)) / 4
		if n = bytes.LastIndex(p, boxHorizontal); n == -1 {
			err = fmt.Errorf("invalid syntax: %q", p)
			return
		}
		name = p[n:]
		if n = bytes.Index(name, boxSpace); n == -1 {
			err = fmt.Errorf("invalid syntax: %q", p)
			return
		}
		name = name[n+1:]
		return
	}
	Tab = func(p []byte) (depth int, name []byte, err error) {
		depth = bytes.Count(p, []byte{'\t'})
		name = p[depth:]
		return
	}
}

// Tree builds FS.Tree from given reader using CustomTree callback for parsing
// node's name and its depth in the tree.
func (ct CustomTree) Tree(r io.Reader) (fs FS, err error) {
	var (
		dir       = Directory{}
		buf       = bufio.NewReader(r)
		glob      []Directory
		name      []byte
		prevName  []byte
		depth     int
		prevDepth int
	)
	fs.Tree = dir
	line, err := buf.ReadBytes('\n')
	if len(line) == 0 || err == io.EOF {
		err = io.ErrUnexpectedEOF
		return
	}
	if err != nil {
		return
	}
	if len(line) != 1 || line[0] != '.' {
		p := filepath.FromSlash(string(bytes.TrimSpace(line)))
		if err = fs.MkdirAll(p, 0); err != nil {
			return
		}
		// TODO(rjeczalik): make it an exported helper method
		var perr *os.PathError
		if dir, perr = fs.lookup(p); perr != nil {
			err = perr
			return
		}
	}
	glob = append(glob, dir)
	for {
		line, err = buf.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) == 0 {
			io.Copy(ioutil.Discard, buf)
			err, line = io.EOF, nil
		} else {
			depth, name, err = ct(bytes.TrimRightFunc(line, unicode.IsSpace))
		}
		// Skip first iteration.
		if len(prevName) != 0 {
			// Insert the node from previous iteration - node is a directory when
			// a diference of the tree depth > 0, a file otherwise.
			var (
				name  string
				value interface{}
			)
			if bytes.HasSuffix(prevName, []byte{'/'}) {
				name, value = string(bytes.TrimRight(prevName, "/")), Directory{}
			} else {
				name, value = string(prevName), File{}
			}
			switch {
			case depth > prevDepth:
				d := Directory{}
				dir[name], glob, dir = d, append(glob, dir), d
			case depth == prevDepth:
				dir[name] = value
			case depth < prevDepth:
				n := max(len(glob)+depth-prevDepth, 0)
				dir[name], dir, glob = value, glob[n], glob[:n]
			}
		}
		// A node from each iteration is handled on the next one. That's why the
		// error handling is deferred.
		if len(line) == 0 {
			if err == io.EOF {
				err = nil
			}
			return
		}
		prevDepth, prevName = depth, name
	}
}

// UnixTree builds FS.Tree from a buffer that contains tree-like (Unix command) output.
//
// Example:
//
//   var tree = []byte(`.
//   └── dir
//       └── file.txt`)
//
//   fs, _ = memfs.UnixTree(tree)
//   fmt.Printf("%#v\n", fs)
//
//   // Produces:
//   // memfs.FS{Tree: memfs.Directory{"dir": memfs.Directory{"file": memfs.File{}}}}
//
// UnixTree(p) is an short alternative to the Unix.Tree(bytes.NewReader(p)).
func UnixTree(p []byte) (FS, error) {
	return Unix.Tree(bytes.NewReader(p))
}

// TabTree builds FS.Tree from a buffer that contains \t-separated file tree.
//
// Example:
//
//   var tree = []byte(`.
//   dir
//   	file.txt`)
//
//   fs, _ = memfs.TabTree(tree)
//   fmt.Printf("%#v\n", fs)
//   // Produces:
//   // memfs.FS{Tree: memfs.Directory{"dir": memfs.Directory{"file": memfs.File{}}}}
//
// TabTree(p) is an short alternative to the Tab.Tree(bytes.NewReader(p)).
func TabTree(p []byte) (FS, error) {
	return Tab.Tree(bytes.NewReader(p))
}