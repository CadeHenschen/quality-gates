// Package safefile provides filesystem operations bounded by an os.Root.
//
// Walk-based scanners should open one root for the scanned directory and use
// the *At functions with paths relative to that root. For explicit CLI paths,
// the path itself selects the parent directory; operations are then bounded
// to that parent so a final symlink cannot redirect access outside it.
package safefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// OpenRoot opens a filesystem root for operations on paths relative to dir.
func OpenRoot(dir string) (*os.Root, error) {
	return os.OpenRoot(dir)
}

// OpenAt opens a file below root. os.Root rejects paths that escape the root,
// including symlinks whose targets leave it.
func OpenAt(root *os.Root, path string) (*os.File, error) {
	return root.Open(path)
}

// ReadFileAt reads a file below root. path must be relative to root.
func ReadFileAt(root *os.Root, path string) ([]byte, error) {
	f, err := OpenAt(root, path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(f)
	return data, errors.Join(readErr, f.Close())
}

// ReadFile reads a caller-selected input file while preventing a final
// symlink from escaping its selected parent directory.
func ReadFile(path string) ([]byte, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(f)
	return data, errors.Join(readErr, f.Close())
}

// Open opens a caller-selected input file bounded to its selected parent.
func Open(path string) (*os.File, error) {
	root, name, err := openPathRoot(path)
	if err != nil {
		return nil, err
	}
	f, openErr := root.Open(name)
	rootErr := root.Close()
	if openErr != nil || rootErr != nil {
		if f != nil {
			return nil, errors.Join(openErr, rootErr, f.Close())
		}
		return nil, errors.Join(openErr, rootErr)
	}
	return f, nil
}

// Create creates or truncates a caller-selected output file bounded to its
// selected parent directory.
func Create(path string) (*os.File, error) {
	root, name, err := openPathRoot(path)
	if err != nil {
		return nil, err
	}
	f, createErr := root.Create(name)
	rootErr := root.Close()
	if createErr != nil || rootErr != nil {
		if f != nil {
			return nil, errors.Join(createErr, rootErr, f.Close())
		}
		return nil, errors.Join(createErr, rootErr)
	}
	return f, nil
}

func openPathRoot(path string) (*os.Root, string, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}
	return root, filepath.Base(path), nil
}
