// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/armon/go-radix"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/hashicorp/go-multierror"
)

const (
	separator = "/"
)

// Store implements an in-memory Store. We will use the hashicorp/go-radix implementation.
// This pkg provides a mutable radix which gives O(k) lookup and ordered iteration.
type Store struct {
	st *radix.Tree
	mu sync.RWMutex
}

func New() *Store {
	return &Store{st: radix.New()}
}

func getKeyString(i storage.Key) string {
	return strings.Join([]string{i.Namespace(), i.ID()}, separator)
}

func idFromKey(pfx, key string) string {
	return strings.TrimPrefix(key, pfx+separator)
}

func (s *Store) Get(i storage.Item) error {
	key := getKeyString(i)

	s.mu.RLock()
	val, found := s.st.Get(key)
	s.mu.RUnlock()
	if !found {
		return storage.ErrNotFound
	}

	err := i.Unmarshal(val.([]byte))
	if err != nil {
		return fmt.Errorf("failed unmarshaling item: %w", err)
	}

	return nil
}

func (s *Store) Has(k storage.Key) (bool, error) {
	key := getKeyString(k)

	s.mu.RLock()
	_, found := s.st.Get(key)
	s.mu.RUnlock()

	return found, nil
}

func (s *Store) GetSize(k storage.Key) (int, error) {
	key := getKeyString(k)

	s.mu.RLock()
	val, found := s.st.Get(key)
	s.mu.RUnlock()
	if !found {
		return 0, storage.ErrNotFound
	}

	return len(val.([]byte)), nil
}

func (s *Store) Put(i storage.Item) error {
	key := getKeyString(i)

	val, err := i.Marshal()
	if err != nil {
		return fmt.Errorf("failed marshaling item: %w", err)
	}

	s.mu.Lock()
	s.st.Insert(key, val)
	s.mu.Unlock()

	return nil
}

func (s *Store) Delete(i storage.Item) error {
	key := getKeyString(i)

	s.mu.Lock()
	_, deleted := s.st.Delete(key)
	s.mu.Unlock()
	if !deleted {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) Count(k storage.Key) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	s.st.WalkPrefix(k.Namespace(), func(_ string, _ interface{}) bool {
		count++
		return false
	})
	return count, nil
}

func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var retErr *multierror.Error
	prefix := q.Factory().Namespace()

	getNext := func(k string, v interface{}) (*storage.Result, error) {
		for _, filter := range q.Filters {
			if filter(idFromKey(prefix, k), v.([]byte)) {
				return nil, nil
			}
		}
		var res *storage.Result
		switch q.ItemAttribute {
		case storage.QueryItemID, storage.QueryItemSize:
			res = &storage.Result{ID: idFromKey(prefix, k), Size: len(v.([]byte))}
		case storage.QueryItem:
			newItem := q.Factory()
			err := newItem.Unmarshal(v.([]byte))
			if err != nil {
				return nil, fmt.Errorf("failed unmarshaling: %w", err)
			}
			res = &storage.Result{Entry: newItem}
		}
		return res, nil
	}

	switch q.Order {
	case storage.KeyAscendingOrder:
		s.st.WalkPrefix(prefix, func(k string, v interface{}) bool {
			res, err := getNext(k, v)
			if err != nil {
				retErr = multierror.Append(retErr, err)
				return true
			}
			if res != nil {
				stop, err := fn(*res)
				if err != nil {
					retErr = multierror.Append(retErr, fmt.Errorf("failed in iterate function: %w", err))
					return true
				}
				return stop
			}
			return false
		})
	case storage.KeyDescendingOrder:
		// currently there is no optimal way to do reverse iteration. We can efficiently do forward
		// iteration. So we have two options, first is to reduce time complexity by compromising
		// on space complexity. So we keep track of keys and values during forward iteration
		// to do a simple reverse iteration. Other option is to reduce space complexity by keeping
		// track of only keys during forward iteration, then use Get to read the value on reverse
		// iteration. This would involve additional complexity of doing a Get on reverse iteration.
		// For now, inmem implementation is not meant to work for large datasets, so first option
		// is chosen.
		results := make([]storage.Result, 0)
		s.st.WalkPrefix(prefix, func(k string, v interface{}) bool {
			res, err := getNext(k, v)
			if err != nil {
				retErr = multierror.Append(retErr, err)
				return true
			}
			if res != nil {
				results = append(results, *res)
			}
			return false
		})
		if retErr.ErrorOrNil() != nil {
			break
		}
		for i := len(results) - 1; i >= 0; i-- {
			stop, err := fn(results[i])
			if err != nil {
				return fmt.Errorf("failed in iterate function: %w", err)
			}
			if stop {
				break
			}
		}
	}
	return retErr.ErrorOrNil()
}

func (s *Store) Close() error {
	return nil
}

func (s *Store) Batch(ctx context.Context) (storage.Batch, error) {
	return &Batch{
		ctx:   ctx,
		store: s,
	}, nil
}

type Batch struct {
	ctx   context.Context
	items []storage.Item
	store *Store
}

func (i *Batch) Put(item storage.Item) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}
	i.items = append(i.items, item)
	return nil
}

func (i *Batch) Delete(key storage.Key) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}
	for index, rem := range i.items {
		if rem.ID() == key.ID() {
			i.items = append(i.items[:index], i.items[index+1:]...)
		}
	}

	return nil
}

func (i *Batch) Commit() error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}
	for _, item := range i.items {
		i.store.Put(item)
	}

	return nil
}
