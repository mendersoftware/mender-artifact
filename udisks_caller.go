// Copyright 2017 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package main

import (
	"sync"

	"github.com/godbus/dbus"
	"github.com/pkg/errors"
)

func registerDeviceCallback(conn *dbus.Conn, signalCh chan *dbus.Signal,
	wg *sync.WaitGroup, cb func(*dbus.Signal) error) error {
	call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal',path='/org/freedesktop/UDisks2',interface='org.freedesktop.DBus.ObjectManager'")

	if call.Err != nil {
		return errors.Wrap(call.Err, "can not register devices signal")
	} else {
		go func() {
			conn.Signal(signalCh)

			for {
				msg, ok := <-signalCh
				if !ok {
					conn.RemoveSignal(signalCh)
					wg.Done()
					return
				}

				if err := cb(msg); err != nil {
					conn.RemoveSignal(signalCh)
					wg.Done()
					return
				}
			}
		}()
	}
	return nil
}

func mountFile(conn *dbus.Conn, fd dbus.UnixFD) (string, error) {
	obj := conn.Object("org.freedesktop.UDisks2", "/org/freedesktop/UDisks2/Manager")
	call := obj.Call("org.freedesktop.UDisks2.Manager.LoopSetup", 0, fd,
		map[string]dbus.Variant{"read-only": dbus.MakeVariant(false)})
	if call.Err != nil {
		return "", errors.Wrap(call.Err, "can not loop mount device")
	}

	var s string
	if err := call.Store(&s); err != nil {
		return "", errors.Wrap(err, "error getting loop device")
	}
	return s, nil
}

func contains(s []string, e string) bool {
	for _, v := range s {
		if v == e {
			return true
		}
	}
	return false
}

func getDeviceProperties(conn *dbus.Conn, device string,
	props []string) (map[string]interface{}, error) {

	prop := make(map[string]dbus.Variant)
	obj := conn.Object("org.freedesktop.UDisks2", dbus.ObjectPath(device))
	call := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.freedesktop.UDisks2.Block")
	if call.Err != nil {
		return nil, errors.Wrapf(call.Err, "can not get device [%s] properties", device)
	}
	if err := call.Store(prop); err != nil {
		return nil, errors.Wrapf(err, "can not store device [%s] properties", device)
	}

	requested := make(map[string]interface{}, 0)

	for k, v := range prop {
		if contains(props, k) {
			requested[k] = v.Value()
		}
	}

	prop = make(map[string]dbus.Variant)
	obj = conn.Object("org.freedesktop.UDisks2", dbus.ObjectPath(device))
	call = obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.freedesktop.UDisks2.Filesystem")
	if call.Err != nil {
		return nil, errors.Wrapf(call.Err, "can not get device [%s] properties", device)
	}
	if err := call.Store(prop); err != nil {
		return nil, errors.Wrapf(err, "can not store device [%s] properties", device)
	}

	for k, v := range prop {
		if contains(props, k) {
			requested[k] = v.Value()
		}
	}

	return requested, nil
}
