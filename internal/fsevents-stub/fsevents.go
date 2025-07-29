// +build darwin

package fsevents

import (
	"errors"
	"time"
)

type EventFlags uint32
type CreateFlags uint32

type Event struct {
	Path  string
	Flags EventFlags
}

type EventStream struct {
	Paths   []string
	Events  <-chan []Event
	Latency time.Duration
	Flags   CreateFlags
	EventID uint64
	stop    func()
}

func (es *EventStream) Start() error {
	return nil
}

func (es *EventStream) Stop() {}

func (es *EventStream) Restart() {}

func (es *EventStream) Resume() bool { return false }

func GetDeviceUUID(deviceID int32) (string, error) {
	return "", errors.New("fsevents not supported")
}

type fsEventStreamRef uintptr
type fsDispatchQueueRef uintptr

func New(dev int32, since uint64, latency time.Duration, flags CreateFlags, paths ...string) *EventStream {
	return &EventStream{
		Paths:   paths,
		Events:  make(<-chan []Event),
		Latency: latency,
	}
}

const (
	ItemCreated = EventFlags(1 << iota)
	ItemRemoved
	ItemInodeMetaMod
	ItemRenamed
	ItemModified
	ItemFinderInfoMod
	ItemChangeOwner
	ItemXattrMod
	ItemIsFile
	ItemIsDir
	ItemIsSymlink
	ItemOwnerGroupMod
	ItemIsHardlink
	ItemIsLastHardlink
	ItemCloned
)

const (
	FileEvents = CreateFlags(1 << iota)
	NoDefer
	WatchRoot
	IgnoreSelf
	MarkSelf
)

func flush() {}
func stop() {}

var EventStreamEventFlagItemIsFile = ItemIsFile
var EventStreamEventFlagItemIsDir = ItemIsDir
var EventStreamEventFlagItemModified = ItemModified
var EventStreamEventFlagItemCreated = ItemCreated

func LatestEventID() uint64 {
	return ^uint64(0)
}