package sources

type SourceConfig interface{}

type Source interface {
	Name() string
	Start() error
	Stop()
}
