package pkg

import (
	"errors"
	"fmt"
	"time"
)

// EventStoreListOptions holds all configuration for listing events.
type EventStoreListOptions struct {
	TenantUuid    string
	AggregateUuid string
	Domains       []string
	DataType      string
	Before        int64
	After         int64
	Offset        int64
	Limit         int64
	OrderBy       string
	Ascending     bool
}

// EventStoreListOption defines a function type for setting options when listing events.
type EventStoreListOption func(opt *EventStoreListOptions) (*EventStoreListOptions, error)

// --- Basic Options ---

// WithTenantUuid filters events by tenant.
func WithTenantUuid(tenantUuid string) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.TenantUuid = tenantUuid
		return opt, nil
	}
}

// WithAggregateUuid filters events by aggregate.
func WithAggregateUuid(aggregateUuid string) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.AggregateUuid = aggregateUuid
		return opt, nil
	}
}

// WithDomains filters events by domain(s).
func WithDomains(domains ...string) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.Domains = append(opt.Domains, domains...)
		return opt, nil
	}
}

// WithDataType filters events by data type.
func WithDataType(dataType string) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.DataType = dataType
		return opt, nil
	}
}

// WithLimit sets the maximum number of events to return.
func WithLimit(limit int64) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		if limit <= 0 {
			return nil, errors.New("limit must be positive")
		}
		if limit > 10000 {
			return nil, errors.New("limit exceeds maximum (10000)")
		}
		opt.Limit = limit
		return opt, nil
	}
}

// WithOffset sets the offset for pagination.
func WithOffset(offset int64) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		if offset < 0 {
			return nil, errors.New("offset cannot be negative")
		}
		opt.Offset = offset
		return opt, nil
	}
}

// WithAscending sets the sort order.
func WithAscending(ascending bool) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.Ascending = ascending
		return opt, nil
	}
}

// WithOrderBy sets the field to order by.
func WithOrderBy(orderBy string) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.OrderBy = orderBy
		return opt, nil
	}
}

// --- Convenience Options (combining multiple settings) ---

// WithPagination combines offset and limit into a page-based API.
func WithPagination(page, pageSize int64) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		if page < 1 {
			return nil, errors.New("page must be >= 1")
		}
		if pageSize <= 0 {
			return nil, errors.New("pageSize must be positive")
		}
		if pageSize > 10000 {
			return nil, errors.New("pageSize exceeds maximum (10000)")
		}
		opt.Offset = (page - 1) * pageSize
		opt.Limit = pageSize
		return opt, nil
	}
}

// WithTimeRange filters events within a time range.
func WithTimeRange(start, end time.Time) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		if end.Before(start) {
			return nil, fmt.Errorf("invalid time range: end (%v) is before start (%v)", end, start)
		}
		opt.After = start.UnixNano()
		opt.Before = end.UnixNano()
		return opt, nil
	}
}

// WithAfter filters events created after a timestamp.
func WithAfter(t time.Time) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.After = t.UnixNano()
		return opt, nil
	}
}

// WithBefore filters events created before a timestamp.
func WithBefore(t time.Time) EventStoreListOption {
	return func(opt *EventStoreListOptions) (*EventStoreListOptions, error) {
		opt.Before = t.UnixNano()
		return opt, nil
	}
}

// --- Preset Options (reusable configurations) ---

// RecentEventsDefaults returns options for fetching recent events.
func RecentEventsDefaults() []EventStoreListOption {
	return []EventStoreListOption{
		WithLimit(50),
		WithAscending(false), // newest first
		WithOrderBy("created_at"),
	}
}

// AuditLogDefaults returns options suitable for audit log queries.
func AuditLogDefaults() []EventStoreListOption {
	return []EventStoreListOption{
		WithLimit(1000),
		WithAscending(true), // chronological order
		WithOrderBy("created_at"),
	}
}
