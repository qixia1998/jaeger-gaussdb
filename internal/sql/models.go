// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0

package sql

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sqlc-dev/pqtype"
)

type Spankind string

const (
	SpankindServer      Spankind = "server"
	SpankindClient      Spankind = "client"
	SpankindUnspecified Spankind = "unspecified"
	SpankindProducer    Spankind = "producer"
	SpankindConsumer    Spankind = "consumer"
	SpankindEphemeral   Spankind = "ephemeral"
	SpankindInternal    Spankind = "internal"
)

func (e *Spankind) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = Spankind(s)
	case string:
		*e = Spankind(s)
	default:
		return fmt.Errorf("unsupported scan type for Spankind: %T", src)
	}
	return nil
}

type NullSpankind struct {
	Spankind Spankind
	Valid    bool // Valid is true if Spankind is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullSpankind) Scan(value interface{}) error {
	if value == nil {
		ns.Spankind, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.Spankind.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullSpankind) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.Spankind), nil
}

type Operation struct {
	ID        int64
	Name      string
	ServiceID int64
	Kind      Spankind
}

type Service struct {
	ID   int64
	Name string
}

type Span struct {
	HackID      int64
	SpanID      []byte
	TraceID     []byte
	OperationID int64
	Flags       int64
	StartTime   time.Time
	Duration    int64
	Tags        pqtype.NullRawMessage
	ServiceID   int64
	ProcessID   string
	ProcessTags json.RawMessage
	Warnings    []string
	Logs        pqtype.NullRawMessage
	Kind        Spankind
	Refs        json.RawMessage
}
