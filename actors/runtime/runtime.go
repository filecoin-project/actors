package runtime

import (
	"bytes"
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	cid "github.com/ipfs/go-cid"

	"github.com/filecoin-project/specs-actors/actors/runtime/proof"
)

// Specifies importance of message, LogLevel numbering is consistent with the uber-go/zap package.
type LogLevel int

const (
	// DebugLevel logs are typically voluminous, and are usually disabled in
	// production.
	DEBUG LogLevel = iota - 1
	// InfoLevel is the default logging priority.
	INFO
	// WarnLevel logs are more important than Info, but don't need individual
	// human review.
	WARN
	// ErrorLevel logs are high-priority. If an application is running smoothly,
	// it shouldn't generate any error-level logs.
	ERROR
)

// Runtime is the VM's internal runtime object.
// this is everything that is accessible to actors, beyond parameters.
type Runtime interface {
	// Information related to the current message being executed.
	// When an actor invokes a method on another actor as a sub-call, these values reflect
	// the sub-call context, rather than the top-level context.
	Message() Message

	// The current chain epoch number. The genesis block has epoch zero.
	CurrEpoch() abi.ChainEpoch

	// Satisfies the requirement that every exported actor method must invoke at least one caller validation
	// method before returning, without making any assertions about the caller.
	ValidateImmediateCallerAcceptAny()

	// Validates that the immediate caller's address exactly matches one of a set of expected addresses,
	// aborting if it does not.
	// The caller address is always normalized to an ID address, so expected addresses must be
	// ID addresses to have any expectation of passing validation.
	ValidateImmediateCallerIs(addrs ...addr.Address)

	// Validates that the immediate caller is an actor with code CID matching one of a set of
	// expected CIDs, aborting if it does not.
	ValidateImmediateCallerType(types ...cid.Cid)

	// The balance of the receiver. Always >= zero.
	CurrentBalance() abi.TokenAmount

	// Resolves an address of any protocol to an ID address (via the Init actor's table).
	// This allows resolution of externally-provided SECP, BLS, or actor addresses to the canonical form.
	// If the argument is an ID address it is returned directly.
	ResolveAddress(address addr.Address) (addr.Address, bool)

	// Look up the code ID at an actor address.
	// The address will be resolved as if via ResolveAddress, if necessary, so need not be an ID-address.
	GetActorCodeCID(addr addr.Address) (ret cid.Cid, ok bool)

	// GetRandomnessFromBeacon returns a (pseudo)random byte array drawing from a random beacon at a prior epoch.
	// The beacon value is combined with the personalization tag, epoch number, and explicitly provided entropy.
	// The personalization tag may be any int64 value.
	// The epoch must be less than the current epoch. The epoch may be negative, in which case
	// it addresses the beacon value from genesis block.
	// The entropy may be any byte array, or nil.
	GetRandomnessFromBeacon(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness

	// GetRandomnessFromTickets samples randomness from the ticket chain. Randomess
	// sampled through this method is unique per potential fork, and as a
	// result, processes relying on this randomness are tied to whichever fork
	// they choose.
	// See GetRandomnessFromBeacon for notes about the personalization tag, epoch, and entropy.
	GetRandomnessFromTickets(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness

	// Provides a handle for the actor's state object.
	State() StateHandle

	Store() Store

	// Sends a message to another actor, returning the exit code and return value envelope.
	// If the invoked method does not return successfully, its state changes (and that of any messages it sent in turn)
	// will be rolled back.
	// The result is never a bare nil, but may be (a wrapper of) adt.Empty.
	Send(toAddr addr.Address, methodNum abi.MethodNum, params CBORMarshaler, value abi.TokenAmount) (SendReturn, exitcode.ExitCode)

	// Halts execution upon an error from which the receiver cannot recover. The caller will receive the exitcode and
	// an empty return value. State changes made within this call will be rolled back.
	// This method does not return.
	// The provided exit code must be >= exitcode.FirstActorExitCode.
	// The message and args are for diagnostic purposes and do not persist on chain. They should be suitable for
	// passing to fmt.Errorf(msg, args...).
	Abortf(errExitCode exitcode.ExitCode, msg string, args ...interface{})

	// Computes an address for a new actor. The returned address is intended to uniquely refer to
	// the actor even in the event of a chain re-org (whereas an ID-address might refer to a
	// different actor after messages are re-ordered).
	// Always an ActorExec address.
	NewActorAddress() addr.Address

	// Creates an actor with code `codeID` and address `address`, with empty state.
	// May only be called by Init actor.
	// Aborts if the provided address has previously been created.
	CreateActor(codeId cid.Cid, address addr.Address)

	// Deletes the executing actor from the state tree, transferring any balance to beneficiary.
	// Aborts if the beneficiary does not exist or is the calling actor.
	// May only be called by the actor itself.
	DeleteActor(beneficiary addr.Address)

	// Provides the system call interface.
	Syscalls() Syscalls

	// Returns the total token supply in circulation at the beginning of the current epoch.
	// The circulating supply is the sum of:
	// - rewards emitted by the reward actor,
	// - funds vested from lock-ups in the genesis state,
	// less the sum of:
	// - funds burnt,
	// - pledge collateral locked in storage miner actors (recorded in the storage power actor)
	// - deal collateral locked by the storage market actor
	TotalFilCircSupply() abi.TokenAmount

	// Provides a Go context for use by HAMT, etc.
	// The VM is intended to provide an idealised machine abstraction, with infinite storage etc, so this context
	// should not be used by actor code directly.
	Context() context.Context

	// Starts a new tracing span. The span must be End()ed explicitly, typically with a deferred invocation.
	StartSpan(name string) TraceSpan

	// ChargeGas charges specified amount of `gas` for execution.
	// `name` provides information about gas charging point
	// `virtual` sets virtual amount of gas to charge, this amount is not counted
	// toward execution cost. This functionality is used for observing global changes
	// in total gas charged if amount of gas charged was to be changed.
	ChargeGas(name string, gas int64, virtual int64)

	// Note events that may make debugging easier
	Log(level LogLevel, msg string, args ...interface{})
}

// Store defines the storage module exposed to actors.
type Store interface {
	// Retrieves and deserializes an object from the store into `o`. Returns whether successful.
	Get(c cid.Cid, o CBORUnmarshaler) bool
	// Serializes and stores an object, returning its CID.
	Put(x CBORMarshaler) cid.Cid
}

// Message contains information available to the actor about the executing message.
// These values are fixed for the duration of an invocation.
type Message interface {
	// The address of the immediate calling actor. Always an ID-address.
	// If an actor invokes its own method, Caller() == Receiver().
	Caller() addr.Address

	// The address of the actor receiving the message. Always an ID-address.
	Receiver() addr.Address

	// The value attached to the message being processed, implicitly added to CurrentBalance()
	// of Receiver() before method invocation.
	// This value came from Caller().
	ValueReceived() abi.TokenAmount
}

// Pure functions implemented as primitives by the runtime.
type Syscalls interface {
	// Verifies that a signature is valid for an address and plaintext.
	// If the address is a public-key type address, it is used directly.
	// If it's an ID-address, the actor is looked up in state. It must be an account actor, and the
	// public key is obtained from it's state.
	VerifySignature(signature crypto.Signature, signer addr.Address, plaintext []byte) error
	// Hashes input data using blake2b with 256 bit output.
	HashBlake2b(data []byte) [32]byte
	// Computes an unsealed sector CID (CommD) from its constituent piece CIDs (CommPs) and sizes.
	ComputeUnsealedSectorCID(reg abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error)
	// Verifies a sector seal proof.
	VerifySeal(vi proof.SealVerifyInfo) error

	BatchVerifySeals(vis map[address.Address][]proof.SealVerifyInfo) (map[address.Address][]bool, error)

	// Verifies a proof of spacetime.
	VerifyPoSt(vi proof.WindowPoStVerifyInfo) error
	// Verifies that two block headers provide proof of a consensus fault:
	// - both headers mined by the same actor
	// - headers are different
	// - first header is of the same or lower epoch as the second
	// - the headers provide evidence of a fault (see the spec for the different fault types).
	// The parameters are all serialized block headers. The third "extra" parameter is consulted only for
	// the "parent grinding fault", in which case it must be the sibling of h1 (same parent tipset) and one of the
	// blocks in an ancestor of h2.
	// Returns nil and an error if the headers don't prove a fault.
	VerifyConsensusFault(h1, h2, extra []byte) (*ConsensusFault, error)
}

// The return type from a message send from one actor to another. This abstracts over the internal representation of
// the return, in particular whether it has been serialized to bytes or just passed through.
// Production code is expected to de/serialize, but test and other code may pass the value straight through.
type SendReturn interface {
	Into(CBORUnmarshaler) error
}

// Provides (minimal) tracing facilities to actor code.
type TraceSpan interface {
	// Ends the span
	End()
}

// StateHandle provides mutable, exclusive access to actor state.
type StateHandle interface {
	// Create initializes the state object.
	// This is only valid in a constructor function and when the state has not yet been initialized.
	Create(obj CBORMarshaler)

	// Readonly loads a readonly copy of the state into the argument.
	//
	// Any modification to the state is illegal and will result in an abort.
	Readonly(obj CBORUnmarshaler)

	// Transaction loads a mutable version of the state into the `obj` argument and protects
	// the execution from side effects (including message send).
	//
	// The second argument is a function which allows the caller to mutate the state.
	//
	// If the state is modified after this function returns, execution will abort.
	//
	// The gas cost of this method is that of a Store.Put of the mutated state object.
	//
	// Note: the Go signature is not ideal due to lack of type system power.
	//
	// # Usage
	// ```go
	// var state SomeState
	// rt.State().Transaction(&state, func() {
	// 	// make some changes
	// 	state.ImLoaded = true
	// })
	// // state.ImLoaded = false // BAD!! state is readonly outside the lambda, it will panic
	// ```
	Transaction(obj CBORer, f func())
}

// Result of checking two headers for a consensus fault.
type ConsensusFault struct {
	// Address of the miner at fault (always an ID address).
	Target addr.Address
	// Epoch of the fault, which is the higher epoch of the two blocks causing it.
	Epoch abi.ChainEpoch
	// Type of fault.
	Type ConsensusFaultType
}

type ConsensusFaultType int64

const (
	//ConsensusFaultNone             ConsensusFaultType = 0
	ConsensusFaultDoubleForkMining ConsensusFaultType = 1
	ConsensusFaultParentGrinding   ConsensusFaultType = 2
	ConsensusFaultTimeOffsetMining ConsensusFaultType = 3
)

// These interfaces are intended to match those from whyrusleeping/cbor-gen, such that code generated from that
// system is automatically usable here (but not mandatory).
type CBORMarshaler interface {
	MarshalCBOR(w io.Writer) error
}

type CBORUnmarshaler interface {
	UnmarshalCBOR(r io.Reader) error
}

type CBORer interface {
	CBORMarshaler
	CBORUnmarshaler
}

// Wraps already-serialized bytes as CBOR-marshalable.
type CBORBytes []byte

func (b CBORBytes) MarshalCBOR(w io.Writer) error {
	_, err := w.Write(b)
	return err
}

func (b *CBORBytes) UnmarshalCBOR(r io.Reader) error {
	var c bytes.Buffer
	_, err := c.ReadFrom(r)
	*b = c.Bytes()
	return err
}
