package tests

import (
	"testing"
	"time"

	"github.com/matrix-org/complement-crypto/rust/matrix_sdk_ffi"

	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
)

func TestCreateRoom(t *testing.T) {
	deployment := Deploy(t)
	// pre-register alice and bob
	csapiAlice := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "alice",
		Password:        "testfromrustsdk",
	})
	csapiBob := deployment.Register(t, "hs1", helpers.RegistrationOpts{
		LocalpartSuffix: "bob",
		Password:        "testfromrustsdk",
	})
	ss := deployment.SlidingSyncURL(t)

	// Rust SDK testing below
	// ----------------------

	// Alice creates an encrypted room
	ab := matrix_sdk_ffi.NewClientBuilder().HomeserverUrl(csapiAlice.BaseURL).SlidingSyncProxy(&ss)
	alice, err := ab.Build()
	must.NotError(t, "client builder failed to build", err)
	must.NotError(t, "failed to login", alice.Login(csapiAlice.UserID, "testfromrustsdk", nil, nil))
	roomName := "Rust SDK Test"
	roomID, err := alice.CreateRoom(matrix_sdk_ffi.CreateRoomParameters{
		Name:        &roomName,
		Visibility:  matrix_sdk_ffi.RoomVisibilityPublic,
		Preset:      matrix_sdk_ffi.RoomPresetPublicChat,
		IsEncrypted: true,
	})
	must.NotError(t, "failed to create room", err)
	must.NotEqual(t, roomID, "", "empty room id")
	t.Logf("created room %s", roomID)
	wantMsgBody := "Hello world"

	// Alice starts syncing
	aliceSync, err := alice.SyncService().FinishBlocking()
	must.NotError(t, "failed to make sync service", err)
	go aliceSync.StartBlocking()
	defer aliceSync.StopBlocking()
	time.Sleep(time.Second)

	// Alice gets the room she created
	t.Logf("alice getting rooms")
	aliceRooms := alice.Rooms()
	must.Equal(t, len(aliceRooms), 1, "room missing from Rooms()")
	aliceRoom := aliceRooms[0]
	must.Equal(t, aliceRoom.Id(), roomID, "mismatched room IDs")
	enc, err := aliceRoom.IsEncrypted()
	must.NotError(t, "failed to check if room is encrypted", err)
	must.Equal(t, enc, true, "room is not encrypted when it should be")
	// we need a timeline listener before we can send messages
	aliceRoom.AddTimelineListenerBlocking(&timelineListener{fn: func(diff []*matrix_sdk_ffi.TimelineDiff) {

	}})

	// Alice invites Bob
	must.NotError(t, "failed to invite bob", aliceRoom.InviteUserById(csapiBob.UserID))

	// Bob starts syncing
	bb := matrix_sdk_ffi.NewClientBuilder().HomeserverUrl(csapiBob.BaseURL).SlidingSyncProxy(&ss)
	bob, err := bb.Build()
	must.NotError(t, "client builder failed to build", err)
	must.NotError(t, "failed to login", bob.Login(csapiBob.UserID, "testfromrustsdk", nil, nil))
	bobSync, err := bob.SyncService().FinishBlocking()
	must.NotError(t, "failed to make sync service", err)
	go bobSync.StartBlocking()
	defer bobSync.StopBlocking()
	time.Sleep(time.Second)

	// Bob gets the room he was invited to
	t.Logf("bob getting rooms")
	bobRooms := bob.Rooms()
	must.Equal(t, len(bobRooms), 1, "room missing from Rooms()")
	bobRoom := bobRooms[0]
	must.Equal(t, bobRoom.Id(), roomID, "mismatched room IDs")
	// we need a timeline listener before we can send messages
	var bobMsgs []string
	waiter := helpers.NewWaiter()
	bobRoom.AddTimelineListenerBlocking(&timelineListener{fn: func(diff []*matrix_sdk_ffi.TimelineDiff) {
		var items []*matrix_sdk_ffi.TimelineItem
		for _, d := range diff {
			t.Logf("diff %v", d.Change())
			switch d.Change() {
			case matrix_sdk_ffi.TimelineChangeInsert:
				insertData := d.Insert()
				if insertData == nil {
					continue
				}
				items = append(items, insertData.Item)
			case matrix_sdk_ffi.TimelineChangeAppend:
				appendItems := d.Append()
				if appendItems == nil {
					continue
				}
				items = append(items, *appendItems...)
			case matrix_sdk_ffi.TimelineChangePushBack:
				pbData := d.PushBack()
				if pbData == nil {
					continue
				}
				items = append(items, *pbData)
			case matrix_sdk_ffi.TimelineChangeSet:
				setData := d.Set()
				if setData == nil {
					continue
				}
				items = append(items, setData.Item)
			}
		}
		for _, item := range items {
			t.Logf("handle item %v", item.FmtDebug())
			ev := item.AsEvent()
			if ev == nil {
				continue
			}
			evv := *ev
			msg := evv.Content().AsMessage()
			if msg == nil {
				continue
			}
			msgg := *msg
			bobMsgs = append(bobMsgs, msgg.Body())
			t.Logf("bob got item: %s", msgg.Body())
			if msgg.Body() == wantMsgBody {
				waiter.Finish()
			}
		}
	}})

	// Bob accepts the invite
	must.NotError(t, "bob failed to join room", bobRoom.Join())

	// Alice sends a message
	aliceRoom.Send(matrix_sdk_ffi.MessageEventContentFromHtml(wantMsgBody, wantMsgBody))

	// Bob receives the message
	waiter.Wait(t, time.Second)
}

type timelineListener struct {
	fn func(diff []*matrix_sdk_ffi.TimelineDiff)
}

func (l *timelineListener) OnUpdate(diff []*matrix_sdk_ffi.TimelineDiff) {
	l.fn(diff)
}
