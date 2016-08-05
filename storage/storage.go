package storage

type Repository struct {
	ID          string
	FullName    string
	AccessToken string
	Plugins     []string
	GitURL      string
}

type RepositoryWriter interface {
	Store(Repository) error
}

type RepositoryReader interface {
	Load() ([]Repository, error)
}

type RepositoryReaderWriter interface {
	RepositoryReader
	RepositoryWriter
}
