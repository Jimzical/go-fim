package agent

import "go.etcd.io/bbolt"

// deferClose returns a deferred closer that propagates a db.Close error into
// err only when the surrounding function hasn't already failed.
// Usage: defer deferClose(db, &err)()
func deferClose(db *bbolt.DB, err *error) func() {
	return func() {
		if cerr := db.Close(); cerr != nil && *err == nil {
			*err = cerr
		}
	}
}
