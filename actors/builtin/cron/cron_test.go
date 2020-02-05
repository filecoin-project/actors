package cron_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	cron "github.com/filecoin-project/specs-actors/actors/builtin/cron"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	mock "github.com/filecoin-project/specs-actors/support/mock"
	tutil "github.com/filecoin-project/specs-actors/support/testing"
)

func TestConstructor(t *testing.T) {
	actor := cronHarness{cron.CronActor{}, t}

	receiver := tutil.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver).WithCaller(builtin.SystemActorAddr, builtin.SystemActorCodeID)

	t.Run("construct with empty entries", func(t *testing.T) {
		rt := builder.Build(t)

		var nilCronEntries = []cron.CronTableEntry(nil)
		actor.constructAndVerify(rt, nilCronEntries...)

		var st cron.CronActorState
		rt.GetState(&st)
		assert.Equal(t, nilCronEntries, st.Entries)
	})

	t.Run("construct with non-empty entries", func(t *testing.T) {
		rt := builder.Build(t)

		var cronEntries = []cron.CronTableEntry{
			{Receiver: tutil.NewIDAddr(t, 1001), MethodNum: abi.MethodNum(1001)},
			{Receiver: tutil.NewIDAddr(t, 1002), MethodNum: abi.MethodNum(1002)},
			{Receiver: tutil.NewIDAddr(t, 1003), MethodNum: abi.MethodNum(1003)},
			{Receiver: tutil.NewIDAddr(t, 1004), MethodNum: abi.MethodNum(1004)},
		}
		actor.constructAndVerify(rt, cronEntries...)

		var st cron.CronActorState
		rt.GetState(&st)
		assert.Equal(t, cronEntries, st.Entries)
	})
}

func TestEpochTick(t *testing.T) {
	actor := cronHarness{cron.CronActor{}, t}

	receiver := tutil.NewIDAddr(t, 100)
	builder := mock.NewBuilder(context.Background(), receiver).WithCaller(builtin.SystemActorAddr, builtin.SystemActorCodeID)

	t.Run("epoch tick with empty entries", func(t *testing.T) {
		rt := builder.Build(t)

		var nilCronEntries = []cron.CronTableEntry(nil)
		actor.constructAndVerify(rt, nilCronEntries...)
		actor.epochTickAndVerify(rt)
	})

	t.Run("epoch tick with non-empty entries", func(t *testing.T) {
		rt := builder.Build(t)

		entry1 := cron.CronTableEntry{Receiver: tutil.NewIDAddr(t, 1001), MethodNum: abi.MethodNum(1001)}
		entry2 := cron.CronTableEntry{Receiver: tutil.NewIDAddr(t, 1002), MethodNum: abi.MethodNum(1002)}
		entry3 := cron.CronTableEntry{Receiver: tutil.NewIDAddr(t, 1003), MethodNum: abi.MethodNum(1003)}
		entry4 := cron.CronTableEntry{Receiver: tutil.NewIDAddr(t, 1004), MethodNum: abi.MethodNum(1004)}

		actor.constructAndVerify(rt, entry1, entry2, entry3, entry4)
		// exit code should not matter
		rt.ExpectSend(entry1.Receiver, entry1.MethodNum, adt.EmptyValue{}, big.Zero(), nil, exitcode.Ok)
		rt.ExpectSend(entry2.Receiver, entry2.MethodNum, adt.EmptyValue{}, big.Zero(), nil, exitcode.ErrIllegalArgument)
		rt.ExpectSend(entry3.Receiver, entry3.MethodNum, adt.EmptyValue{}, big.Zero(), nil, exitcode.ErrInsufficientFunds)
		rt.ExpectSend(entry4.Receiver, entry4.MethodNum, adt.EmptyValue{}, big.Zero(), nil, exitcode.ErrForbidden)
		actor.epochTickAndVerify(rt)
	})

}

type cronHarness struct {
	cron.CronActor
	t testing.TB
}

func (h *cronHarness) constructAndVerify(rt *mock.Runtime, entries ...cron.CronTableEntry) {
	params := cron.ConstructorParams{Entries: entries}
	rt.ExpectValidateCallerAddr(builtin.SystemActorAddr)
	ret := rt.Call(h.Constructor, &params).(*adt.EmptyValue)
	assert.Equal(h.t, &adt.EmptyValue{}, ret)
	rt.Verify()
}

func (h *cronHarness) epochTickAndVerify(rt *mock.Runtime) {
	rt.ExpectValidateCallerAddr(builtin.SystemActorAddr)
	ret := rt.Call(h.EpochTick, &adt.EmptyValue{}).(*adt.EmptyValue)
	assert.Equal(h.t, &adt.EmptyValue{}, ret)
	rt.Verify()
}
