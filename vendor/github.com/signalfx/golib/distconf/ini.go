package distconf

import (
	"github.com/signalfx/golib/errors"
	ini "github.com/vaughan0/go-ini"
)

type propertyFileDisco struct {
	noopCloser
	filename string
	file     ini.File
}

func (p *propertyFileDisco) Get(key string) ([]byte, error) {
	ret, ok := p.file.Get("", key)
	if !ok {
		return nil, nil
	}
	return []byte(ret), nil
}

// Ini creates a backing config reader that reads properties from an Ini file
func Ini(filename string) (Reader, error) {
	file, err := ini.LoadFile(filename)
	if err != nil {
		return nil, errors.Annotatef(err, "Unable to open file %s", filename)
	}
	return &propertyFileDisco{
		filename: filename,
		file:     file,
	}, nil

}

// IniLoader is a helper for loading from Ini files
func IniLoader(filename string) BackingLoader {
	return BackingLoaderFunc(func() (Reader, error) {
		return Ini(filename)
	})
}
