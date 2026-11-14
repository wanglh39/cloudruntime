package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type ResourceStore struct {
	mu        sync.RWMutex
	resources map[string]map[string]*ResourceObject
	version   int64
	watchers  map[string][]chan *WatchEvent
	watcherMu sync.Mutex
}

type ResourceObject struct {
	Meta   ResourceMeta `json:"metadata"`
	Spec   interface{}  `json:"spec"`
	Status interface{}  `json:"status"`
}

func NewResourceStore() *ResourceStore {
	return &ResourceStore{
		resources: make(map[string]map[string]*ResourceObject),
		watchers:  make(map[string][]chan *WatchEvent),
	}
}

func (s *ResourceStore) nextVersion() int64 {
	return atomic.AddInt64(&s.version, 1)
}

func (s *ResourceStore) Create(kind, name string, spec interface{}) (*ResourceObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resources[kind] == nil {
		s.resources[kind] = make(map[string]*ResourceObject)
	}

	if _, exists := s.resources[kind][name]; exists {
		return nil, fmt.Errorf("%s %s already exists", kind, name)
	}

	obj := &ResourceObject{
		Meta: ResourceMeta{
			Name:            name,
			ResourceVersion: s.nextVersion(),
			CreatedAt:       time.Now(),
		},
		Spec:   spec,
		Status: nil,
	}

	s.resources[kind][name] = obj
	s.notifyWatchers(kind, EventAdded, obj)
	return obj, nil
}

func (s *ResourceStore) Get(kind, name string) (*ResourceObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.resources[kind] == nil {
		return nil, fmt.Errorf("%s %s not found", kind, name)
	}

	obj, ok := s.resources[kind][name]
	if !ok {
		return nil, fmt.Errorf("%s %s not found", kind, name)
	}
	return obj, nil
}

func (s *ResourceStore) List(kind string) []*ResourceObject {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.resources[kind] == nil {
		return nil
	}

	result := make([]*ResourceObject, 0, len(s.resources[kind]))
	for _, obj := range s.resources[kind] {
		result = append(result, obj)
	}
	return result
}

func (s *ResourceStore) Update(kind, name string, spec interface{}) (*ResourceObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resources[kind] == nil {
		return nil, fmt.Errorf("%s %s not found", kind, name)
	}

	obj, ok := s.resources[kind][name]
	if !ok {
		return nil, fmt.Errorf("%s %s not found", kind, name)
	}

	obj.Spec = spec
	obj.Meta.ResourceVersion = s.nextVersion()
	s.notifyWatchers(kind, EventModified, obj)
	return obj, nil
}

func (s *ResourceStore) UpdateStatus(kind, name string, status interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resources[kind] == nil {
		return fmt.Errorf("%s %s not found", kind, name)
	}

	obj, ok := s.resources[kind][name]
	if !ok {
		return fmt.Errorf("%s %s not found", kind, name)
	}

	obj.Status = status
	obj.Meta.ResourceVersion = s.nextVersion()
	s.notifyWatchers(kind, EventModified, obj)
	return nil
}

func (s *ResourceStore) Delete(kind, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resources[kind] == nil {
		return fmt.Errorf("%s %s not found", kind, name)
	}

	obj, ok := s.resources[kind][name]
	if !ok {
		return fmt.Errorf("%s %s not found", kind, name)
	}

	delete(s.resources[kind], name)
	s.notifyWatchers(kind, EventDeleted, obj)
	return nil
}

func (s *ResourceStore) Watch(kind string) chan *WatchEvent {
	s.watcherMu.Lock()
	defer s.watcherMu.Unlock()

	ch := make(chan *WatchEvent, 100)
	s.watchers[kind] = append(s.watchers[kind], ch)
	return ch
}

func (s *ResourceStore) StopWatch(kind string, ch chan *WatchEvent) {
	s.watcherMu.Lock()
	defer s.watcherMu.Unlock()

	watchers := s.watchers[kind]
	for i, w := range watchers {
		if w == ch {
			s.watchers[kind] = append(watchers[:i], watchers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (s *ResourceStore) notifyWatchers(kind string, eventType EventType, obj *ResourceObject) {
	s.watcherMu.Lock()
	defer s.watcherMu.Unlock()

	event := &WatchEvent{
		Type:   eventType,
		Object: obj,
	}

	for _, ch := range s.watchers[kind] {
		select {
		case ch <- event:
		default:
		}
	}
}

type APIServer struct {
	store *ResourceStore
	mux   *http.ServeMux
}

func NewAPIServer(store *ResourceStore) *APIServer {
	server := &APIServer{
		store: store,
		mux:   http.NewServeMux(),
	}
	server.registerRoutes()
	return server
}

func (s *APIServer) registerRoutes() {
	for _, kind := range []string{"sandboxes", "nodes", "volumes", "functions", "policies"} {
		s.mux.HandleFunc("/api/v1/"+kind, s.handleCollection(kind))
		s.mux.HandleFunc("/api/v1/"+kind+"/", s.handleSingle(kind))
	}
}

func (s *APIServer) handleCollection(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case "GET":
			if r.URL.Query().Get("watch") == "true" {
				s.handleWatch(w, r, kind)
				return
			}
			list := s.store.List(kind)
			json.NewEncoder(w).Encode(list)

		case "POST":
			var obj ResourceObject
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			created, err := s.store.Create(kind, obj.Meta.Name, obj.Spec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(created)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (s *APIServer) handleSingle(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		name := r.URL.Path[len("/api/v1/"+kind+"/"):]
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case "GET":
			obj, err := s.store.Get(kind, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(obj)

		case "PUT":
			var obj ResourceObject
			if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			updated, err := s.store.Update(kind, name, obj.Spec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(updated)

		case "DELETE":
			if err := s.store.Delete(kind, name); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (s *APIServer) handleWatch(w http.ResponseWriter, r *http.Request, kind string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	ch := s.store.Watch(kind)
	defer s.store.StopWatch(kind, ch)

	for _, obj := range s.store.List(kind) {
		event := &WatchEvent{Type: EventAdded, Object: obj}
		json.NewEncoder(w).Encode(event)
		flusher.Flush()
	}

	for {
		select {
		case event := <-ch:
			json.NewEncoder(w).Encode(event)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *APIServer) Start(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}
