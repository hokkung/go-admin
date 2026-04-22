package admin

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type errorEnvelope struct {
	Error *Error `json:"error"`
}

func writeError(c *fiber.Ctx, err error) error {
	e, ok := err.(*Error)
	if !ok {
		e = &Error{Status: 500, Code: "INTERNAL", Message: err.Error()}
	}
	return c.Status(e.Status).JSON(errorEnvelope{Error: e})
}

// Mount attaches admin routes (POST /<entity>.<action>) onto the given
// fiber app or router group supplied by the caller.
func (a *Admin) Mount(router fiber.Router) {
	router.Post("/:path", a.dispatch)
}

func (a *Admin) dispatch(c *fiber.Ctx) error {
	path := c.Params("path")
	if path == "" {
		path = strings.TrimPrefix(c.Path(), "/")
	}
	if path == "" {
		return writeError(c, ErrInvalidEntity)
	}
	dot := strings.Index(path, ".")
	if dot < 0 {
		return writeError(c, badRequest("path must be /<entity>.<action>"))
	}
	entityName, action := path[:dot], path[dot+1:]
	if entityName == systemEntity {
		return a.dispatchSystem(c, action)
	}
	meta, ok := a.lookup(entityName)
	if !ok {
		return writeError(c, ErrInvalidEntity)
	}

	switch action {
	case "create":
		return a.handleCreate(c, meta)
	case "get":
		return a.handleGet(c, meta)
	case "update":
		return a.handleUpdate(c, meta)
	case "delete":
		return a.handleDelete(c, meta)
	case "list":
		return a.handleList(c, meta)
	case "metadata":
		return a.handleMetadata(c, meta)
	default:
		if ca, ok := meta.actions[action]; ok && ca.Handler != nil {
			return ca.Handler(c)
		}
		return writeError(c, ErrInvalidAction)
	}
}

func (a *Admin) handleCreate(c *fiber.Ctx, m *entityMeta) error {
	instance := m.newInstance()
	if len(c.Body()) > 0 {
		if err := json.Unmarshal(c.Body(), instance.Interface()); err != nil {
			return writeError(c, badRequest("invalid json body: "+err.Error()))
		}
	}
	val := instance.Elem()
	if m.idGenerator != nil {
		idField := val.FieldByIndex(m.idField.index)
		if idField.IsZero() {
			id, err := m.idGenerator(metaFor(m))
			if err != nil {
				return writeError(c, err)
			}
			if err := m.setID(val, id); err != nil {
				return writeError(c, err)
			}
		}
	}
	if err := runValidators(m, val); err != nil {
		return writeError(c, validationError(err))
	}
	created, err := m.storage.Create(c.Context(), metaFor(m), val)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(created.Interface())
}

type idRequest struct {
	ID json.RawMessage `json:"id"`
}

func (a *Admin) extractID(m *entityMeta, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, badRequest("id is required")
	}
	ptr := reflect.New(m.idField.goType)
	if err := json.Unmarshal(raw, ptr.Interface()); err != nil {
		return nil, badRequest("invalid id: " + err.Error())
	}
	return ptr.Elem().Interface(), nil
}

func (a *Admin) handleGet(c *fiber.Ctx, m *entityMeta) error {
	var req idRequest
	if len(c.Body()) > 0 {
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return writeError(c, badRequest("invalid json body: "+err.Error()))
		}
	}
	id, err := a.extractID(m, req.ID)
	if err != nil {
		return writeError(c, err)
	}
	v, err := m.storage.Get(c.Context(), metaFor(m), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(v.Interface())
}

func (a *Admin) handleUpdate(c *fiber.Ctx, m *entityMeta) error {
	var raw map[string]json.RawMessage
	if len(c.Body()) > 0 {
		if err := json.Unmarshal(c.Body(), &raw); err != nil {
			return writeError(c, badRequest("invalid json body: "+err.Error()))
		}
	}
	idRaw, ok := raw["id"]
	if !ok {
		return writeError(c, badRequest("id is required"))
	}
	id, err := a.extractID(m, idRaw)
	if err != nil {
		return writeError(c, err)
	}

	existing, err := m.storage.Get(c.Context(), metaFor(m), id)
	if err != nil {
		return writeError(c, err)
	}

	merged := m.newInstance()
	merged.Elem().Set(reflect.Indirect(existing))
	delete(raw, "id")
	rebuilt, mErr := json.Marshal(raw)
	if mErr != nil {
		return writeError(c, badRequest("invalid update body"))
	}
	if err := json.Unmarshal(rebuilt, merged.Interface()); err != nil {
		return writeError(c, badRequest("invalid update body: "+err.Error()))
	}

	if err := runValidators(m, merged.Elem()); err != nil {
		return writeError(c, validationError(err))
	}
	updated, err := m.storage.Update(c.Context(), metaFor(m), id, merged.Elem())
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(updated.Interface())
}

func (a *Admin) handleDelete(c *fiber.Ctx, m *entityMeta) error {
	var req idRequest
	if len(c.Body()) > 0 {
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return writeError(c, badRequest("invalid json body: "+err.Error()))
		}
	}
	id, err := a.extractID(m, req.ID)
	if err != nil {
		return writeError(c, err)
	}
	if err := m.storage.Delete(c.Context(), metaFor(m), id); err != nil {
		return writeError(c, err)
	}
	return c.JSON(fiber.Map{"deleted": true, "id": id})
}

func (a *Admin) handleMetadata(c *fiber.Ctx, m *entityMeta) error {
	return c.JSON(fiber.Map{"data": m.buildMetadata(requestBase(c))})
}

// requestBase returns the fully-qualified URL prefix under which admin routes
// are reachable, e.g. "http://host:8080/admin/". It derives the mount prefix
// from the current request path by stripping the final "<entity>.<action>"
// segment, so callers can compose any admin route as base+"<entity>.<action>".
func requestBase(c *fiber.Ctx) string {
	p := c.Path()
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[:i+1]
	}
	return c.BaseURL() + p
}

// systemEntity is the reserved entity name for framework-level endpoints
// such as the catalog of all registered entities.
const systemEntity = "admin"

func (a *Admin) dispatchSystem(c *fiber.Ctx, action string) error {
	switch action {
	case "entities":
		return a.handleEntities(c)
	default:
		return writeError(c, ErrInvalidAction)
	}
}

func (a *Admin) handleEntities(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": a.buildCatalog(requestBase(c))})
}

func (a *Admin) handleList(c *fiber.Ctx, m *entityMeta) error {
	var q ListQuery
	if len(c.Body()) > 0 {
		if err := json.Unmarshal(c.Body(), &q); err != nil {
			return writeError(c, badRequest("invalid json body: "+err.Error()))
		}
	}
	if err := validateQuery(m, &q); err != nil {
		return writeError(c, err)
	}
	result, err := m.storage.List(c.Context(), metaFor(m), q)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(result)
}
