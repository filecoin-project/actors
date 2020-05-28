// Code generated by github.com/whyrusleeping/cbor-gen. DO NOT EDIT.

package power

import (
	"fmt"
	"io"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	peer "github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"
)

var _ = xerrors.Errorf

func (t *State) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{137}); err != nil {
		return err
	}

	// t.TotalRawBytePower (big.Int) (struct)
	if err := t.TotalRawBytePower.MarshalCBOR(w); err != nil {
		return err
	}

	// t.TotalQualityAdjPower (big.Int) (struct)
	if err := t.TotalQualityAdjPower.MarshalCBOR(w); err != nil {
		return err
	}

	// t.TotalPledgeCollateral (big.Int) (struct)
	if err := t.TotalPledgeCollateral.MarshalCBOR(w); err != nil {
		return err
	}

	// t.MinerCount (int64) (int64)
	if t.MinerCount >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.MinerCount))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.MinerCount)-1)); err != nil {
			return err
		}
	}

	// t.CronEventQueue (cid.Cid) (struct)

	if err := cbg.WriteCid(w, t.CronEventQueue); err != nil {
		return xerrors.Errorf("failed to write cid field t.CronEventQueue: %w", err)
	}

	// t.LastEpochTick (abi.ChainEpoch) (int64)
	if t.LastEpochTick >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.LastEpochTick))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.LastEpochTick)-1)); err != nil {
			return err
		}
	}

	// t.Claims (cid.Cid) (struct)

	if err := cbg.WriteCid(w, t.Claims); err != nil {
		return xerrors.Errorf("failed to write cid field t.Claims: %w", err)
	}

	// t.NumMinersMeetingMinPower (int64) (int64)
	if t.NumMinersMeetingMinPower >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.NumMinersMeetingMinPower))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.NumMinersMeetingMinPower)-1)); err != nil {
			return err
		}
	}

	// t.ProofValidationBatch (cid.Cid) (struct)

	if t.ProofValidationBatch == nil {
		if _, err := w.Write(cbg.CborNull); err != nil {
			return err
		}
	} else {
		if err := cbg.WriteCid(w, *t.ProofValidationBatch); err != nil {
			return xerrors.Errorf("failed to write cid field t.ProofValidationBatch: %w", err)
		}
	}

	return nil
}

func (t *State) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 9 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.TotalRawBytePower (big.Int) (struct)

	{

		if err := t.TotalRawBytePower.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.TotalRawBytePower: %w", err)
		}

	}
	// t.TotalQualityAdjPower (big.Int) (struct)

	{

		if err := t.TotalQualityAdjPower.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.TotalQualityAdjPower: %w", err)
		}

	}
	// t.TotalPledgeCollateral (big.Int) (struct)

	{

		if err := t.TotalPledgeCollateral.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.TotalPledgeCollateral: %w", err)
		}

	}
	// t.MinerCount (int64) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.MinerCount = int64(extraI)
	}
	// t.CronEventQueue (cid.Cid) (struct)

	{

		c, err := cbg.ReadCid(br)
		if err != nil {
			return xerrors.Errorf("failed to read cid field t.CronEventQueue: %w", err)
		}

		t.CronEventQueue = c

	}
	// t.LastEpochTick (abi.ChainEpoch) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.LastEpochTick = abi.ChainEpoch(extraI)
	}
	// t.Claims (cid.Cid) (struct)

	{

		c, err := cbg.ReadCid(br)
		if err != nil {
			return xerrors.Errorf("failed to read cid field t.Claims: %w", err)
		}

		t.Claims = c

	}
	// t.NumMinersMeetingMinPower (int64) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.NumMinersMeetingMinPower = int64(extraI)
	}
	// t.ProofValidationBatch (cid.Cid) (struct)

	{

		pb, err := br.PeekByte()
		if err != nil {
			return err
		}
		if pb == cbg.CborNull[0] {
			var nbuf [1]byte
			if _, err := br.Read(nbuf[:]); err != nil {
				return err
			}
		} else {

			c, err := cbg.ReadCid(br)
			if err != nil {
				return xerrors.Errorf("failed to read cid field t.ProofValidationBatch: %w", err)
			}

			t.ProofValidationBatch = &c
		}

	}
	return nil
}

func (t *Claim) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{130}); err != nil {
		return err
	}

	// t.RawBytePower (big.Int) (struct)
	if err := t.RawBytePower.MarshalCBOR(w); err != nil {
		return err
	}

	// t.QualityAdjPower (big.Int) (struct)
	if err := t.QualityAdjPower.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *Claim) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.RawBytePower (big.Int) (struct)

	{

		if err := t.RawBytePower.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.RawBytePower: %w", err)
		}

	}
	// t.QualityAdjPower (big.Int) (struct)

	{

		if err := t.QualityAdjPower.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.QualityAdjPower: %w", err)
		}

	}
	return nil
}

func (t *CronEvent) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{130}); err != nil {
		return err
	}

	// t.MinerAddr (address.Address) (struct)
	if err := t.MinerAddr.MarshalCBOR(w); err != nil {
		return err
	}

	// t.CallbackPayload ([]uint8) (slice)
	if len(t.CallbackPayload) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("Byte array in field t.CallbackPayload was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(t.CallbackPayload)))); err != nil {
		return err
	}
	if _, err := w.Write(t.CallbackPayload); err != nil {
		return err
	}
	return nil
}

func (t *CronEvent) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.MinerAddr (address.Address) (struct)

	{

		if err := t.MinerAddr.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.MinerAddr: %w", err)
		}

	}
	// t.CallbackPayload ([]uint8) (slice)

	maj, extra, err = cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if extra > cbg.ByteArrayMaxLen {
		return fmt.Errorf("t.CallbackPayload: byte array too large (%d)", extra)
	}
	if maj != cbg.MajByteString {
		return fmt.Errorf("expected byte array")
	}
	t.CallbackPayload = make([]byte, extra)
	if _, err := io.ReadFull(br, t.CallbackPayload); err != nil {
		return err
	}
	return nil
}

func (t *CreateMinerParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{132}); err != nil {
		return err
	}

	// t.Owner (address.Address) (struct)
	if err := t.Owner.MarshalCBOR(w); err != nil {
		return err
	}

	// t.Worker (address.Address) (struct)
	if err := t.Worker.MarshalCBOR(w); err != nil {
		return err
	}

	// t.SealProofType (abi.RegisteredProof) (int64)
	if t.SealProofType >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.SealProofType))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.SealProofType)-1)); err != nil {
			return err
		}
	}

	// t.Peer (peer.ID) (string)
	if len(t.Peer) > cbg.MaxLength {
		return xerrors.Errorf("Value in field t.Peer was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajTextString, uint64(len(t.Peer)))); err != nil {
		return err
	}
	if _, err := w.Write([]byte(t.Peer)); err != nil {
		return err
	}
	return nil
}

func (t *CreateMinerParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 4 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Owner (address.Address) (struct)

	{

		if err := t.Owner.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Owner: %w", err)
		}

	}
	// t.Worker (address.Address) (struct)

	{

		if err := t.Worker.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Worker: %w", err)
		}

	}
	// t.SealProofType (abi.RegisteredProof) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.SealProofType = abi.RegisteredProof(extraI)
	}
	// t.Peer (peer.ID) (string)

	{
		sval, err := cbg.ReadString(br)
		if err != nil {
			return err
		}

		t.Peer = peer.ID(sval)
	}
	return nil
}

func (t *DeleteMinerParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{129}); err != nil {
		return err
	}

	// t.Miner (address.Address) (struct)
	if err := t.Miner.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *DeleteMinerParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 1 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Miner (address.Address) (struct)

	{

		if err := t.Miner.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Miner: %w", err)
		}

	}
	return nil
}

func (t *EnrollCronEventParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{130}); err != nil {
		return err
	}

	// t.EventEpoch (abi.ChainEpoch) (int64)
	if t.EventEpoch >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.EventEpoch))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.EventEpoch)-1)); err != nil {
			return err
		}
	}

	// t.Payload ([]uint8) (slice)
	if len(t.Payload) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("Byte array in field t.Payload was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(t.Payload)))); err != nil {
		return err
	}
	if _, err := w.Write(t.Payload); err != nil {
		return err
	}
	return nil
}

func (t *EnrollCronEventParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.EventEpoch (abi.ChainEpoch) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.EventEpoch = abi.ChainEpoch(extraI)
	}
	// t.Payload ([]uint8) (slice)

	maj, extra, err = cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if extra > cbg.ByteArrayMaxLen {
		return fmt.Errorf("t.Payload: byte array too large (%d)", extra)
	}
	if maj != cbg.MajByteString {
		return fmt.Errorf("expected byte array")
	}
	t.Payload = make([]byte, extra)
	if _, err := io.ReadFull(br, t.Payload); err != nil {
		return err
	}
	return nil
}

func (t *OnSectorTerminateParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{130}); err != nil {
		return err
	}

	// t.TerminationType (power.SectorTermination) (int64)
	if t.TerminationType >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.TerminationType))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.TerminationType)-1)); err != nil {
			return err
		}
	}

	// t.Weights ([]power.SectorStorageWeightDesc) (slice)
	if len(t.Weights) > cbg.MaxLength {
		return xerrors.Errorf("Slice value in field t.Weights was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajArray, uint64(len(t.Weights)))); err != nil {
		return err
	}
	for _, v := range t.Weights {
		if err := v.MarshalCBOR(w); err != nil {
			return err
		}
	}
	return nil
}

func (t *OnSectorTerminateParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.TerminationType (power.SectorTermination) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.TerminationType = SectorTermination(extraI)
	}
	// t.Weights ([]power.SectorStorageWeightDesc) (slice)

	maj, extra, err = cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("t.Weights: array too large (%d)", extra)
	}

	if maj != cbg.MajArray {
		return fmt.Errorf("expected cbor array")
	}

	if extra > 0 {
		t.Weights = make([]SectorStorageWeightDesc, extra)
	}

	for i := 0; i < int(extra); i++ {

		var v SectorStorageWeightDesc
		if err := v.UnmarshalCBOR(br); err != nil {
			return err
		}

		t.Weights[i] = v
	}

	return nil
}

func (t *OnSectorModifyWeightDescParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{130}); err != nil {
		return err
	}

	// t.PrevWeight (power.SectorStorageWeightDesc) (struct)
	if err := t.PrevWeight.MarshalCBOR(w); err != nil {
		return err
	}

	// t.NewWeight (power.SectorStorageWeightDesc) (struct)
	if err := t.NewWeight.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *OnSectorModifyWeightDescParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.PrevWeight (power.SectorStorageWeightDesc) (struct)

	{

		if err := t.PrevWeight.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.PrevWeight: %w", err)
		}

	}
	// t.NewWeight (power.SectorStorageWeightDesc) (struct)

	{

		if err := t.NewWeight.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.NewWeight: %w", err)
		}

	}
	return nil
}

func (t *OnSectorProveCommitParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{129}); err != nil {
		return err
	}

	// t.Weight (power.SectorStorageWeightDesc) (struct)
	if err := t.Weight.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *OnSectorProveCommitParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 1 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Weight (power.SectorStorageWeightDesc) (struct)

	{

		if err := t.Weight.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Weight: %w", err)
		}

	}
	return nil
}

func (t *OnFaultBeginParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{129}); err != nil {
		return err
	}

	// t.Weights ([]power.SectorStorageWeightDesc) (slice)
	if len(t.Weights) > cbg.MaxLength {
		return xerrors.Errorf("Slice value in field t.Weights was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajArray, uint64(len(t.Weights)))); err != nil {
		return err
	}
	for _, v := range t.Weights {
		if err := v.MarshalCBOR(w); err != nil {
			return err
		}
	}
	return nil
}

func (t *OnFaultBeginParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 1 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Weights ([]power.SectorStorageWeightDesc) (slice)

	maj, extra, err = cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("t.Weights: array too large (%d)", extra)
	}

	if maj != cbg.MajArray {
		return fmt.Errorf("expected cbor array")
	}

	if extra > 0 {
		t.Weights = make([]SectorStorageWeightDesc, extra)
	}

	for i := 0; i < int(extra); i++ {

		var v SectorStorageWeightDesc
		if err := v.UnmarshalCBOR(br); err != nil {
			return err
		}

		t.Weights[i] = v
	}

	return nil
}

func (t *OnFaultEndParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{129}); err != nil {
		return err
	}

	// t.Weights ([]power.SectorStorageWeightDesc) (slice)
	if len(t.Weights) > cbg.MaxLength {
		return xerrors.Errorf("Slice value in field t.Weights was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajArray, uint64(len(t.Weights)))); err != nil {
		return err
	}
	for _, v := range t.Weights {
		if err := v.MarshalCBOR(w); err != nil {
			return err
		}
	}
	return nil
}

func (t *OnFaultEndParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 1 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.Weights ([]power.SectorStorageWeightDesc) (slice)

	maj, extra, err = cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("t.Weights: array too large (%d)", extra)
	}

	if maj != cbg.MajArray {
		return fmt.Errorf("expected cbor array")
	}

	if extra > 0 {
		t.Weights = make([]SectorStorageWeightDesc, extra)
	}

	for i := 0; i < int(extra); i++ {

		var v SectorStorageWeightDesc
		if err := v.UnmarshalCBOR(br); err != nil {
			return err
		}

		t.Weights[i] = v
	}

	return nil
}

func (t *CreateMinerReturn) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{130}); err != nil {
		return err
	}

	// t.IDAddress (address.Address) (struct)
	if err := t.IDAddress.MarshalCBOR(w); err != nil {
		return err
	}

	// t.RobustAddress (address.Address) (struct)
	if err := t.RobustAddress.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *CreateMinerReturn) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.IDAddress (address.Address) (struct)

	{

		if err := t.IDAddress.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.IDAddress: %w", err)
		}

	}
	// t.RobustAddress (address.Address) (struct)

	{

		if err := t.RobustAddress.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.RobustAddress: %w", err)
		}

	}
	return nil
}

func (t *MinerConstructorParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{132}); err != nil {
		return err
	}

	// t.OwnerAddr (address.Address) (struct)
	if err := t.OwnerAddr.MarshalCBOR(w); err != nil {
		return err
	}

	// t.WorkerAddr (address.Address) (struct)
	if err := t.WorkerAddr.MarshalCBOR(w); err != nil {
		return err
	}

	// t.SealProofType (abi.RegisteredProof) (int64)
	if t.SealProofType >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.SealProofType))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.SealProofType)-1)); err != nil {
			return err
		}
	}

	// t.PeerId (peer.ID) (string)
	if len(t.PeerId) > cbg.MaxLength {
		return xerrors.Errorf("Value in field t.PeerId was too long")
	}

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajTextString, uint64(len(t.PeerId)))); err != nil {
		return err
	}
	if _, err := w.Write([]byte(t.PeerId)); err != nil {
		return err
	}
	return nil
}

func (t *MinerConstructorParams) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 4 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.OwnerAddr (address.Address) (struct)

	{

		if err := t.OwnerAddr.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.OwnerAddr: %w", err)
		}

	}
	// t.WorkerAddr (address.Address) (struct)

	{

		if err := t.WorkerAddr.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.WorkerAddr: %w", err)
		}

	}
	// t.SealProofType (abi.RegisteredProof) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.SealProofType = abi.RegisteredProof(extraI)
	}
	// t.PeerId (peer.ID) (string)

	{
		sval, err := cbg.ReadString(br)
		if err != nil {
			return err
		}

		t.PeerId = peer.ID(sval)
	}
	return nil
}

func (t *SectorStorageWeightDesc) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{132}); err != nil {
		return err
	}

	// t.SectorSize (abi.SectorSize) (uint64)

	if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.SectorSize))); err != nil {
		return err
	}

	// t.Duration (abi.ChainEpoch) (int64)
	if t.Duration >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.Duration))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.Duration)-1)); err != nil {
			return err
		}
	}

	// t.DealWeight (big.Int) (struct)
	if err := t.DealWeight.MarshalCBOR(w); err != nil {
		return err
	}

	// t.VerifiedDealWeight (big.Int) (struct)
	if err := t.VerifiedDealWeight.MarshalCBOR(w); err != nil {
		return err
	}
	return nil
}

func (t *SectorStorageWeightDesc) UnmarshalCBOR(r io.Reader) error {
	br := cbg.GetPeeker(r)

	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 4 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.SectorSize (abi.SectorSize) (uint64)

	{

		maj, extra, err = cbg.CborReadHeader(br)
		if err != nil {
			return err
		}
		if maj != cbg.MajUnsignedInt {
			return fmt.Errorf("wrong type for uint64 field")
		}
		t.SectorSize = abi.SectorSize(extra)

	}
	// t.Duration (abi.ChainEpoch) (int64)
	{
		maj, extra, err := cbg.CborReadHeader(br)
		var extraI int64
		if err != nil {
			return err
		}
		switch maj {
		case cbg.MajUnsignedInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 positive overflow")
			}
		case cbg.MajNegativeInt:
			extraI = int64(extra)
			if extraI < 0 {
				return fmt.Errorf("int64 negative oveflow")
			}
			extraI = -1 - extraI
		default:
			return fmt.Errorf("wrong type for int64 field: %d", maj)
		}

		t.Duration = abi.ChainEpoch(extraI)
	}
	// t.DealWeight (big.Int) (struct)

	{

		if err := t.DealWeight.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.DealWeight: %w", err)
		}

	}
	// t.VerifiedDealWeight (big.Int) (struct)

	{

		if err := t.VerifiedDealWeight.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.VerifiedDealWeight: %w", err)
		}

	}
	return nil
}
