package distconf

import (
	"os"
	"strings"
)

type cmdDisco struct {
	noopCloser
	prefix string
	source []string
}

func (p *cmdDisco) Get(key string) ([]byte, error) {
	argPrefix := p.prefix + key + "="
	for _, arg := range p.source {
		if !strings.HasPrefix(arg, argPrefix) {
			continue
		}
		argSuffix := arg[len(argPrefix):]
		return []byte(argSuffix), nil
	}
	return nil, nil
}

// Cmd creates a backing reader that reads config varaibles from command line parameters with a prefix
func Cmd(prefix string) Reader {
	return &cmdDisco{
		prefix: prefix,
		source: os.Args,
	}
}

// CmdLoader is a loading helper for command line config variables
func CmdLoader(prefix string) BackingLoader {
	return BackingLoaderFunc(func() (Reader, error) {
		return Cmd(prefix), nil
	})
}
