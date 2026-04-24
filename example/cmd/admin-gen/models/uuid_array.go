package models

import (
	"database/sql/driver"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type UUIDArray []uuid.UUID

func (a *UUIDArray) Scan(src interface{}) error {
	if src == nil {
		*a = UUIDArray{}
		return nil
	}
	var strs []string
	if err := pq.Array(&strs).Scan(src); err != nil {
		return err
	}
	res := make(UUIDArray, len(strs))
	for i, s := range strs {
		id, err := uuid.Parse(s)
		if err != nil {
			return fmt.Errorf("invalid uuid: %w", err)
		}
		res[i] = id
	}
	*a = res
	return nil
}

func (a UUIDArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	strs := make([]string, len(a))
	for i, u := range a {
		strs[i] = u.String()
	}
	return pq.Array(strs).Value()
}
