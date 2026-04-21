package admin

import (
	"fmt"
	"sort"
	"sync"
)

type Admin struct {
	mu       sync.RWMutex
	entities map[string]*entityMeta
}

func New() *Admin {
	return &Admin{entities: map[string]*entityMeta{}}
}

type entityOptions struct {
	name        string
	displayName string
	storage     Storage
	listConfig  ListConfig
	actions     []CustomAction
	idGenerator IDGenerator
}

type Option func(*entityOptions)

func WithName(name string) Option {
	return func(o *entityOptions) { o.name = name }
}

func WithDisplayName(name string) Option {
	return func(o *entityOptions) { o.displayName = name }
}

func WithStorage(s Storage) Option {
	return func(o *entityOptions) { o.storage = s }
}

func WithListConfig(c ListConfig) Option {
	return func(o *entityOptions) { o.listConfig = c }
}

func WithAction(a CustomAction) Option {
	return func(o *entityOptions) { o.actions = append(o.actions, a) }
}

// WithIDGenerator installs a function that produces an id for create requests
// when the client omitted one (or supplied the zero value). Pair with
// admin.AutoIncrement() or admin.AutoUUID() for the common cases.
func WithIDGenerator(g IDGenerator) Option {
	return func(o *entityOptions) { o.idGenerator = g }
}

func (a *Admin) Register(model any, opts ...Option) error {
	o := entityOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	meta, err := newEntityMeta(model, o)
	if err != nil {
		return err
	}
	if meta.name == systemEntity {
		return fmt.Errorf("admin: entity name %q is reserved (use admin.WithName to override)", systemEntity)
	}
	meta.displayName = o.displayName
	meta.listConfig = o.listConfig
	meta.idGenerator = o.idGenerator
	if len(o.actions) > 0 {
		meta.actions = map[string]CustomAction{}
		for _, ca := range o.actions {
			if ca.Name == "" {
				return fmt.Errorf("admin: custom action on %q is missing Name", meta.name)
			}
			if _, clash := reservedActions[ca.Name]; clash {
				return fmt.Errorf("admin: custom action %q on %q clashes with a standard action", ca.Name, meta.name)
			}
			meta.actions[ca.Name] = ca
		}
	}
	if o.storage != nil {
		meta.storage = o.storage
	} else {
		meta.storage = NewMemoryStorage()
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.entities[meta.name]; exists {
		return fmt.Errorf("admin: entity %q already registered", meta.name)
	}
	a.entities[meta.name] = meta
	return nil
}

var reservedActions = map[string]struct{}{
	"create":   {},
	"get":      {},
	"update":   {},
	"delete":   {},
	"list":     {},
	"metadata": {},
}

func (a *Admin) MustRegister(model any, opts ...Option) {
	if err := a.Register(model, opts...); err != nil {
		panic(err)
	}
}

func (a *Admin) Entities() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	names := make([]string, 0, len(a.entities))
	for n := range a.entities {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (a *Admin) lookup(name string) (*entityMeta, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	m, ok := a.entities[name]
	return m, ok
}
