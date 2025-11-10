package files

import (
	"errors"
	"fmt"
	"path"
	"strings"

	hpfs "github.com/hack-pad/hackpadfs"
	hpfsos "github.com/hack-pad/hackpadfs/os"

	"github.com/teapotovh/teapot/lib/tmplstring"
)

var (
	ErrExpectMountFormat = errors.New("expected mount format to be: <vfs>:<src>:<dest>, missing some parts")
)

type mountConfig struct {
	VFS         VFS
	Source      string
	Destination string
}

// parseRawMount parses a mount string format into a configuration struct.
// The expected format is in shape:
// <vfs>:<src>:<dest>
// <vfs> must be a valid files.VFS.
// <src> must include a templated user variable, but will be checked later by
// the files service itself.
func parseRawMount(mount string) (cfg mountConfig, err error) {
	parts := strings.Split(mount, ":")
	if len(parts) != 3 {
		return cfg, ErrExpectMountFormat
	}

	switch parts[0] {
	case VFSOS.String():
		cfg.VFS = VFSOS
	default:
		return cfg, fmt.Errorf("unexpected VFS type: %s", parts[0])
	}

	cfg.Source = path.Clean(parts[1])
	cfg.Destination = path.Clean(parts[2])
	return cfg, nil
}

type mount struct {
	vfs     VFS
	srcTmpl *tmplstring.TMPL[mountSourceParameters]
	dst     string
}

func (m *mount) src(username string) (hpfs.FS, error) {
	path, err := m.srcTmpl.Render(mountSourceParameters{Username: username})
	if err != nil {
		return nil, fmt.Errorf("error while generating mount source: %w", err)
	}

	switch m.vfs {
	case VFSOS:
		fs, err := hpfsos.NewFS().Sub(path)
		if err != nil {
			return nil, fmt.Errorf("error while opening filesystem for %q mount source: %w", m.vfs, err)
		}

		return fs, nil
	default:
		return nil, fmt.Errorf("cannot mount VFS: %s", m.vfs)
	}
}

type mountSourceParameters struct {
	Username string
}

func parseMountConfig(mc mountConfig) (mount, error) {
	srcTmpl, err := tmplstring.NewTMPL[mountSourceParameters](mc.Source)
	if err != nil {
		return mount{}, fmt.Errorf("error while parsing mount source %q: %w", mc.Source, err)
	}

	return mount{
		vfs:     mc.VFS,
		srcTmpl: srcTmpl,
		dst:     mc.Destination,
	}, nil
}
