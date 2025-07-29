// +build darwin,cgo

package fsevents

/*
#cgo LDFLAGS: -framework CoreServices
*/
import "C"