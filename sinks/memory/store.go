// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
)

type Store interface {
	Put(timestamp time.Time, data interface{}) error
	Get(start, end time.Time) []interface{}
}

type Entry struct {
	Timestamp time.Time
	Data      interface{}
}

type realStore struct {
	buffer                    *list.List
	bufferDuration            time.Duration
	oldest                    time.Time
	garbageCollectionDuration time.Duration
	rwLock                    sync.RWMutex
}

func (self *realStore) Put(timestamp time.Time, data interface{}) error {
	if data == nil {
		return fmt.Errorf("cannot store nil data")
	}
	self.rwLock.Lock()
	defer self.rwLock.Unlock()
	if self.buffer.Len() == 0 {
		self.oldest = timestamp
		self.buffer.PushBack(Entry{Timestamp: timestamp, Data: data})
		return nil
		glog.Infof("put pushback: %v, %v", timestamp, data)
	}
	for elem := self.buffer.Back(); elem != nil; elem = elem.Prev() {
		if !timestamp.After(elem.Value.(Entry).Timestamp) {
			glog.Infof("put pushback: %v, %v", timestamp, data)
			self.buffer.InsertBefore(Entry{Timestamp: timestamp, Data: data}, elem)
			if timestamp.Before(self.oldest) {
				self.oldest = timestamp
			}
			return nil
		}
	}
	self.buffer.PushBack((Entry{Timestamp: timestamp, Data: data}))
	return nil
}

func (self *realStore) Get(start, end time.Time) []interface{} {
	self.rwLock.RLock()
	defer self.rwLock.RUnlock()
	if self.buffer.Len() == 0 {
		return nil
	}

	result := []interface{}{}
	for elem := self.buffer.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(Entry)
		if entry.Timestamp.Before(start) || entry.Timestamp.After(end) {
			break
		}
		result = append(result, entry.Data)
	}
	return result
}

func (self *realStore) reapOldData() {
	self.rwLock.Lock()
	defer self.rwLock.Unlock()
	if self.buffer.Len() == 0 || time.Since(self.oldest) <= self.bufferDuration {
		return
	}
	for elem := self.buffer.Front(); elem != nil; elem = self.buffer.Front() {
		if time.Since(elem.Value.(Entry).Timestamp) <= self.bufferDuration {
			self.oldest = elem.Value.(Entry).Timestamp
			break
		}
		self.buffer.Remove(elem)
	}
}

func NewStore(bufferDuration, gcDuration time.Duration) Store {
	store := &realStore{
		buffer:                    list.New(),
		bufferDuration:            bufferDuration,
		garbageCollectionDuration: gcDuration,
	}
	go util.Forever(store.reapOldData, store.garbageCollectionDuration)
	return store
}
