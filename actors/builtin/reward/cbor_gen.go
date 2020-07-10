// Code generated by github.com/whyrusleeping/cbor-gen. DO NOT EDIT.

package reward

import (
	"fmt"
	"io"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"
)

var _ = xerrors.Errorf

func (t *State) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{133}); err != nil {
		return err
	}

	// t.CumsumBaseline (big.Int) (struct)
	if err := t.CumsumBaseline.MarshalCBOR(w); err != nil {
		return err
	}

	// t.CumsumRealized (big.Int) (struct)
	if err := t.CumsumRealized.MarshalCBOR(w); err != nil {
		return err
	}

	// t.EffectiveNetworkTime (abi.ChainEpoch) (int64)
	if t.EffectiveNetworkTime >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.EffectiveNetworkTime))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.EffectiveNetworkTime)-1)); err != nil {
			return err
		}
	}

	// t.ThisEpochReward (big.Int) (struct)
	if err := t.ThisEpochReward.MarshalCBOR(w); err != nil {
		return err
	}

	// t.Epoch (abi.ChainEpoch) (int64)
	if t.Epoch >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.Epoch))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.Epoch)-1)); err != nil {
			return err
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

	if extra != 5 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.CumsumBaseline (big.Int) (struct)

	{

		if err := t.CumsumBaseline.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.CumsumBaseline: %w", err)
		}

	}
	// t.CumsumRealized (big.Int) (struct)

	{

		if err := t.CumsumRealized.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.CumsumRealized: %w", err)
		}

	}
	// t.EffectiveNetworkTime (abi.ChainEpoch) (int64)
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

		t.EffectiveNetworkTime = abi.ChainEpoch(extraI)
	}
	// t.ThisEpochReward (big.Int) (struct)

	{

		if err := t.ThisEpochReward.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.ThisEpochReward: %w", err)
		}

	}
	// t.Epoch (abi.ChainEpoch) (int64)
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

		t.Epoch = abi.ChainEpoch(extraI)
	}
	return nil
}

func (t *AwardBlockRewardParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{132}); err != nil {
		return err
	}

	// t.Miner (address.Address) (struct)
	if err := t.Miner.MarshalCBOR(w); err != nil {
		return err
	}

	// t.Penalty (big.Int) (struct)
	if err := t.Penalty.MarshalCBOR(w); err != nil {
		return err
	}

	// t.GasReward (big.Int) (struct)
	if err := t.GasReward.MarshalCBOR(w); err != nil {
		return err
	}

	// t.WinCount (int64) (int64)
	if t.WinCount >= 0 {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, uint64(t.WinCount))); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(cbg.CborEncodeMajorType(cbg.MajNegativeInt, uint64(-t.WinCount)-1)); err != nil {
			return err
		}
	}
	return nil
}

func (t *AwardBlockRewardParams) UnmarshalCBOR(r io.Reader) error {
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

	// t.Miner (address.Address) (struct)

	{

		if err := t.Miner.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Miner: %w", err)
		}

	}
	// t.Penalty (big.Int) (struct)

	{

		if err := t.Penalty.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.Penalty: %w", err)
		}

	}
	// t.GasReward (big.Int) (struct)

	{

		if err := t.GasReward.UnmarshalCBOR(br); err != nil {
			return xerrors.Errorf("unmarshaling t.GasReward: %w", err)
		}

	}
	// t.WinCount (int64) (int64)
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

		t.WinCount = int64(extraI)
	}
	return nil
}
