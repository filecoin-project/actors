// Code generated by github.com/whyrusleeping/cbor-gen. DO NOT EDIT.

package init

import (
	"fmt"
	"io"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"
)

var _ = xerrors.Errorf

var lengthBufState = []byte{131}

func (t *State) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write(lengthBufState); err != nil {
		return err
	}

	scratch := make([]byte, 9)

	// t.AddressMap (cid.Cid) (struct)

	if err := cbg.WriteCidBuf(scratch, w, t.AddressMap); err != nil {
		return xerrors.Errorf("failed to write cid field t.AddressMap: %w", err)
	}

	// t.NextID (abi.ActorID) (uint64)

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajUnsignedInt, uint64(t.NextID)); err != nil {
		return err
	}

	// t.NetworkName (string) (string)
	if len(t.NetworkName) > cbg.MaxLength {
		return xerrors.Errorf("Value in field t.NetworkName was too long")
	}

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajTextString, uint64(len(t.NetworkName))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, t.NetworkName); err != nil {
		return err
	}
	return nil
}

func (t *State) UnmarshalCBOR(r io.Reader) error {
	*t = State{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 3 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.AddressMap (cid.Cid) (struct)

	{

		c, err := cbg.ReadCid(br)
		if err != nil {
			return xerrors.Errorf("failed to read cid field t.AddressMap: %w", err)
		}

		t.AddressMap = c

	}
	// t.NextID (abi.ActorID) (uint64)

	{

		maj, extra, err = cbg.CborReadHeaderBuf(br, scratch)
		if err != nil {
			return err
		}
		if maj != cbg.MajUnsignedInt {
			return fmt.Errorf("wrong type for uint64 field")
		}
		t.NextID = abi.ActorID(extra)

	}
	// t.NetworkName (string) (string)

	{
		sval, err := cbg.ReadStringBuf(br, scratch)
		if err != nil {
			return err
		}

		t.NetworkName = string(sval)
	}
	return nil
}

var lengthBufConstructorParams = []byte{129}

func (t *ConstructorParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write(lengthBufConstructorParams); err != nil {
		return err
	}

	scratch := make([]byte, 9)

	// t.NetworkName (string) (string)
	if len(t.NetworkName) > cbg.MaxLength {
		return xerrors.Errorf("Value in field t.NetworkName was too long")
	}

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajTextString, uint64(len(t.NetworkName))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, t.NetworkName); err != nil {
		return err
	}
	return nil
}

func (t *ConstructorParams) UnmarshalCBOR(r io.Reader) error {
	*t = ConstructorParams{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 1 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.NetworkName (string) (string)

	{
		sval, err := cbg.ReadStringBuf(br, scratch)
		if err != nil {
			return err
		}

		t.NetworkName = string(sval)
	}
	return nil
}

var lengthBufExecParams = []byte{130}

func (t *ExecParams) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write(lengthBufExecParams); err != nil {
		return err
	}

	scratch := make([]byte, 9)

	// t.CodeCID (cid.Cid) (struct)

	if err := cbg.WriteCidBuf(scratch, w, t.CodeCID); err != nil {
		return xerrors.Errorf("failed to write cid field t.CodeCID: %w", err)
	}

	// t.ConstructorParams ([]uint8) (slice)
	if len(t.ConstructorParams) > cbg.ByteArrayMaxLen {
		return xerrors.Errorf("Byte array in field t.ConstructorParams was too long")
	}

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajByteString, uint64(len(t.ConstructorParams))); err != nil {
		return err
	}

	if _, err := w.Write(t.ConstructorParams); err != nil {
		return err
	}
	return nil
}

func (t *ExecParams) UnmarshalCBOR(r io.Reader) error {
	*t = ExecParams{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}
	if maj != cbg.MajArray {
		return fmt.Errorf("cbor input should be of type array")
	}

	if extra != 2 {
		return fmt.Errorf("cbor input had wrong number of fields")
	}

	// t.CodeCID (cid.Cid) (struct)

	{

		c, err := cbg.ReadCid(br)
		if err != nil {
			return xerrors.Errorf("failed to read cid field t.CodeCID: %w", err)
		}

		t.CodeCID = c

	}
	// t.ConstructorParams ([]uint8) (slice)

	maj, extra, err = cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}

	if extra > cbg.ByteArrayMaxLen {
		return fmt.Errorf("t.ConstructorParams: byte array too large (%d)", extra)
	}
	if maj != cbg.MajByteString {
		return fmt.Errorf("expected byte array")
	}
	t.ConstructorParams = make([]byte, extra)
	if _, err := io.ReadFull(br, t.ConstructorParams); err != nil {
		return err
	}
	return nil
}

var lengthBufExecReturn = []byte{130}

func (t *ExecReturn) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write(lengthBufExecReturn); err != nil {
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

func (t *ExecReturn) UnmarshalCBOR(r io.Reader) error {
	*t = ExecReturn{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
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
