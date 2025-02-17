package ntemplate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/webx-top/echo"
)

type PathAliases map[string][]string

func (p PathAliases) Add(alias, absPath string) PathAliases {
	var err error
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		panic(err)
	}
	if _, ok := p[alias]; !ok {
		p[alias] = []string{}
	}
	if !strings.HasSuffix(absPath, echo.FilePathSeparator) {
		absPath += echo.FilePathSeparator
	}
	p[alias] = append(p[alias], absPath)
	return p
}

func (p PathAliases) ParsePrefix(withAliasPrefixPath string) string {
	rpath, _ := p.ParsePrefixOk(withAliasPrefixPath)
	return rpath
}

func (p PathAliases) ParsePrefixOk(withAliasPrefixPath string) (string, bool) {
	if len(withAliasPrefixPath) < 3 {
		return withAliasPrefixPath, false
	}
	if withAliasPrefixPath[0] == '/' || withAliasPrefixPath[0] == '.' {
		fi, err := os.Stat(withAliasPrefixPath)
		if err == nil && !fi.IsDir() {
			return withAliasPrefixPath, false
		}
		withAliasPrefixPath = withAliasPrefixPath[1:]
	}
	parts := strings.SplitN(withAliasPrefixPath, `/`, 2)
	if len(parts) != 2 {
		return withAliasPrefixPath, false
	}
	alias := parts[0]
	if opaths, ok := p[alias]; ok {
		if len(opaths) == 1 {
			return filepath.Join(opaths[0], withAliasPrefixPath), true
		}
		for _, opath := range opaths {
			_tmpl := filepath.Join(opath, withAliasPrefixPath)
			fi, err := os.Stat(_tmpl)
			if err == nil && !fi.IsDir() {
				return _tmpl, true
			}
		}
	}
	return withAliasPrefixPath, false
}

func (p PathAliases) RestorePrefix(fullpath string) string {
	rpath, _ := p.RestorePrefixOk(fullpath)
	return rpath
}

func (p PathAliases) RestorePrefixOk(fullpath string) (string, bool) {
	for _, absPaths := range p {
		for _, absPath := range absPaths {
			if strings.HasPrefix(fullpath, absPath) {
				return filepath.ToSlash(fullpath[len(absPath):]), true
			}
		}
	}
	return fullpath, false
}

func (p PathAliases) Parse(withAliasTagPath string) string {
	rpath, _ := p.ParseOk(withAliasTagPath)
	return rpath
}

func (p PathAliases) ParseOk(withAliasTagPath string) (string, bool) {
	if len(withAliasTagPath) < 3 || withAliasTagPath[0] != '[' {
		return withAliasTagPath, false
	}
	withAliasTagPath = withAliasTagPath[1:]
	parts := strings.SplitN(withAliasTagPath, `]`, 2)
	if len(parts) != 2 {
		return withAliasTagPath, false
	}
	alias := parts[0]
	rpath := parts[1]
	if opaths, ok := p[alias]; ok {
		if len(opaths) == 1 {
			return filepath.Join(opaths[0], rpath), true
		}
		for _, opath := range opaths {
			_tmpl := filepath.Join(opath, rpath)
			fi, err := os.Stat(_tmpl)
			if err == nil && !fi.IsDir() {
				return _tmpl, true
			}
		}
	}
	return rpath, false
}

func (p PathAliases) Restore(fullpath string) string {
	rpath, _ := p.RestoreOk(fullpath)
	return rpath
}

func (p PathAliases) RestoreOk(fullpath string) (string, bool) {
	for alias, absPaths := range p {
		for _, absPath := range absPaths {
			if strings.HasPrefix(fullpath, absPath) {
				return `[` + alias + `]` + filepath.ToSlash(fullpath[len(absPath):]), true
			}
		}
	}
	return fullpath, false
}
