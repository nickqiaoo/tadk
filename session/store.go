package session

// StoreKind identifies the durability/concurrency profile of a session store.
type StoreKind string

const (
	StoreKindMemory   StoreKind = "memory"
	StoreKindFile     StoreKind = "file"
	StoreKindDatabase StoreKind = "database"
)

// StoreKindProvider is optionally implemented by session services that expose
// their storage profile.
type StoreKindProvider interface {
	StoreKind() StoreKind
}

// KindOf returns the storage profile for a service when available.
func KindOf(service Service) StoreKind {
	if provider, ok := service.(StoreKindProvider); ok {
		return provider.StoreKind()
	}
	return ""
}
