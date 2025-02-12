// package drwmutex provides a DRWMutex, a distributed RWMutex for use when
// there are many readers spread across many cores, and relatively few cores.
// DRWMutex is meant as an almost drop-in replacement for sync.RWMutex.
package drwmutex

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// cpus maps (non-consecutive) CPUID values to integer indices.
var cpus map[uint64]int

// init will construct the cpus map so that CPUIDs can be looked up to
// determine a particular core's lock index.
func init() {
	start := time.Now()
	cpus = map_cpus()
	fmt.Fprintf(os.Stderr, "%d/%d cpus found in %v: %v\n", len(cpus), runtime.NumCPU(), time.Now().Sub(start), cpus)
}

type paddedRWMutex struct {
	_  [8]uint64 // Pad by cache-line size to prevent false sharing.
	mu sync.RWMutex
}

type DRWMutex []paddedRWMutex

// New returns a new, unlocked, distributed RWMutex.
func New() DRWMutex {
	return make(DRWMutex, len(cpus))
}

// Lock takes out an exclusive writer lock similar to sync.Mutex.Lock.
// A writer lock also excludes all readers.
func (mx DRWMutex) Lock() {
	for core := range mx {
		mx[core].mu.Lock()
	}
}

func (mx DRWMutex) TryLock() (ok bool) {
	var core int
	defer func() {
		if ok {
			return
		}
		core--
		for ; core >= 0; core-- {
			mx[core].mu.Unlock()
		}
	}()
	for ; core < len(mx); core++ {
		if !mx[core].mu.TryLock() {
			return false
		}
	}
	return true
}

// Unlock releases an exclusive writer lock similar to sync.Mutex.Unlock.
func (mx DRWMutex) Unlock() {
	for core := range mx {
		mx[core].mu.Unlock()
	}
}

// RLocker returns a sync.Locker presenting Lock() and Unlock() methods that
// take and release a non-exclusive *reader* lock. Note that this call may be
// relatively slow, depending on the underlying system architechture, and so
// its result should be cached if possible.
func (mx DRWMutex) RLocker() sync.Locker {
	return mx[cpus[cpu()]].mu.RLocker()
}

// RLock takes out a non-exclusive reader lock, and returns the lock that was
// taken so that it can later be released.
func (mx DRWMutex) RLock() (l sync.Locker) {
	l = mx[cpus[cpu()]].mu.RLocker()
	l.Lock()
	return
}
