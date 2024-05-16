package golden

import (
	"sync"
)

type UpgradableLock struct {
	uglMutex sync.RWMutex
	upgrades chan func()
}

func NewUpgradableLock() *UpgradableLock {
	return &UpgradableLock{sync.RWMutex{}, make(chan func(), 1)}
}

func (ugl *UpgradableLock) Lock() {
	ugl.uglMutex.Lock()
	select {
	case v, _ := <-ugl.upgrades:
		v()
	default:
	}
}

// MaybeUpgrade() ensures that f() will be the next function called with a
// write lock, if it returns true.  If it returns false, there is some other
// function that will be called first, and the caller must determine how to
// proceed.  If the upgrade attempt was successful, also fires off a lock-
// unlock that will run f() if there are no other writers waiting.  Does not
// release the read lock presumably held by the caller.
func (ugl *UpgradableLock) MaybeUpgrade(f func()) bool {
	select {
	case ugl.upgrades <- f:
		go func() {
			ugl.Lock()
			ugl.Unlock()
		}()
		return true
	default:
		return false
	}
}

func (ugl *UpgradableLock) Unlock() {
	ugl.uglMutex.Unlock()
}

func (ugl *UpgradableLock) RLock() {
	ugl.uglMutex.RLock()
}

func (ugl *UpgradableLock) RUnlock() {
	ugl.uglMutex.RUnlock()
}
