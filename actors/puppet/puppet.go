package puppet

import (
	addr "github.com/filecoin-project/go-address"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	runtime "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

// The Puppet Actor exists to aid testing the runtime and environment in which it's embedded. It provides direct access
// to the runtime methods, including sending arbitrary messages to other actors, without any preconditions or invariants
// to get in the way.
type Actor struct{}

func (a Actor) Exports() []interface{} {
	return []interface{}{
		builtin.MethodConstructor: a.Constructor,
		2:                         a.Send,
	}
}

var _ abi.Invokee = Actor{}

func (a Actor) Constructor(rt runtime.Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()

	rt.State().Create(&State{})
	return nil
}

type SendParams struct {
	To     addr.Address
	Value  abi.TokenAmount
	Method abi.MethodNum
	Params []byte
}

type SendReturn struct {
	Return runtime.CBORBytes
	Code   exitcode.ExitCode
}

func (a Actor) Send(rt runtime.Runtime, params *SendParams) *SendReturn {
	rt.ValidateImmediateCallerAcceptAny()
	ret, code := rt.Send(
		params.To,
		params.Method,
		runtime.CBORBytes(params.Params),
		params.Value,
	)
	var out runtime.CBORBytes
	if err := ret.Into(&out); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to unmarshal send return: %v", err)
	}
	return &SendReturn{
		Return: out,
		Code:   code,
	}
}

type State struct{}

func init() {
	builder := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}
	c, err := builder.Sum([]byte("fil/1/puppet"))
	if err != nil {
		panic(err)
	}
	PuppetActorCodeID = c
}

// The actor code ID & Methods
var PuppetActorCodeID cid.Cid

var MethodsPuppet = struct {
	Constructor abi.MethodNum
	Send        abi.MethodNum
}{builtin.MethodConstructor, 2}
