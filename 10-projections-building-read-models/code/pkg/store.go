package pkg

import (
	"errors"
	"sync"
)

var ErrNotFound = errors.New("not found")

// ReadmodelStoreBackend defines the storage interface for projections
type ReadmodelStoreBackend interface {
	Get(namespace string, key string) (any, error)
	Set(namespace string, key string, value any) error
	Remove(namespace string, key string) error
	Range(namespace string, fn func(key string, value any) bool)
	Reset(namespaces ...string) error
}

// MemoryStoreBackend is the default in-memory implementation
type MemoryStoreBackend struct {
	namespaces sync.Map // namespace -> *sync.Map
}

func NewMemoryStoreBackend() *MemoryStoreBackend {
	return &MemoryStoreBackend{}
}

func (m *MemoryStoreBackend) getNamespace(namespace string) *sync.Map {
	ns, _ := m.namespaces.LoadOrStore(namespace, &sync.Map{})
	return ns.(*sync.Map)
}

func (m *MemoryStoreBackend) Get(namespace, key string) (any, error) {
	ns := m.getNamespace(namespace)
	if value, ok := ns.Load(key); ok {
		return value, nil
	}
	return nil, ErrNotFound
}

func (m *MemoryStoreBackend) Set(namespace, key string, value any) error {
	ns := m.getNamespace(namespace)
	ns.Store(key, value)
	return nil
}

func (m *MemoryStoreBackend) Remove(namespace, key string) error {
	ns := m.getNamespace(namespace)
	ns.Delete(key)
	return nil
}

func (m *MemoryStoreBackend) Range(namespace string, fn func(key string, value any) bool) {
	ns := m.getNamespace(namespace)
	ns.Range(func(k, v any) bool {
		return fn(k.(string), v)
	})
}

func (m *MemoryStoreBackend) Reset(namespaces ...string) error {
	for _, ns := range namespaces {
		m.namespaces.Delete(ns)
	}
	return nil
}

func (m *MemoryStoreBackend) Count(namespace string) int {
	count := 0
	m.Range(namespace, func(key string, value any) bool {
		count++
		return true
	})
	return count
}

// ReadmodelStore is a type-safe wrapper for projections
type ReadmodelStore[T any] struct {
	backend   ReadmodelStoreBackend
	namespace string
}

func NewReadmodelStore[T any](backend ReadmodelStoreBackend, namespace string) *ReadmodelStore[T] {
	return &ReadmodelStore[T]{
		backend:   backend,
		namespace: namespace,
	}
}

func (s *ReadmodelStore[T]) Get(key string) (T, bool, error) {
	var zero T
	value, err := s.backend.Get(s.namespace, key)
	if err != nil {
		if err == ErrNotFound {
			return zero, false, nil
		}
		return zero, false, err
	}
	return value.(T), true, nil
}

func (s *ReadmodelStore[T]) Set(key string, value T) error {
	return s.backend.Set(s.namespace, key, value)
}

func (s *ReadmodelStore[T]) Remove(key string) error {
	return s.backend.Remove(s.namespace, key)
}

func (s *ReadmodelStore[T]) Range(fn func(key string, value T) bool) {
	s.backend.Range(s.namespace, func(key string, value any) bool {
		return fn(key, value.(T))
	})
}

func (s *ReadmodelStore[T]) Count() int {
	count := 0
	s.Range(func(key string, value T) bool {
		count++
		return true
	})
	return count
}
