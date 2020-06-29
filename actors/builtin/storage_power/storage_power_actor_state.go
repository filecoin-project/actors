package storage_power

import (
	"sort"

	addr "github.com/filecoin-project/go-address"
	cid "github.com/ipfs/go-cid"
	errors "github.com/pkg/errors"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	crypto "github.com/filecoin-project/specs-actors/actors/crypto"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type StoragePowerActorState struct {
	TotalNetworkPower abi.StoragePower

	MinerCount  int64
	EscrowTable cid.Cid // BalanceTable (HAMT[address]TokenAmount)

	// Metadata cached for efficient processing of sector/challenge events.

	// A queue of events to be triggered by cron, indexed by epoch.
	CronEventQueue           cid.Cid // Multimap, (HAMT[ChainEpoch]AMT[CronEvent]
	PoStDetectedFaultMiners  cid.Cid // Set, HAMT[addr.Address]struct{}
	ClaimedPower             cid.Cid // Map, HAMT[address]StoragePower
	NumMinersMeetingMinPower int64
}

type CronEvent struct {
	MinerAddr       addr.Address
	CallbackPayload []byte
}

type AddrKey = adt.AddrKey
type IntKey = adt.IntKey

func ConstructState(store adt.Store) (*StoragePowerActorState, error) {
	emptyMap, err := adt.MakeEmptyMap(store)
	if err != nil {
		return nil, err
	}

	return &StoragePowerActorState{
		TotalNetworkPower:        abi.NewStoragePower(0),
		EscrowTable:              emptyMap.Root(),
		CronEventQueue:           emptyMap.Root(),
		PoStDetectedFaultMiners:  emptyMap.Root(),
		ClaimedPower:             emptyMap.Root(),
		NumMinersMeetingMinPower: 0,
	}, nil
}

func (st *StoragePowerActorState) minerNominalPowerMeetsConsensusMinimum(s adt.Store, minerPower abi.StoragePower) (bool, error) {

	// if miner is larger than min power requirement, we're set
	if minerPower.GreaterThanEqual(ConsensusMinerMinPower) {
		return true, nil
	}

	// otherwise, if another miner meets min power requirement, return false
	if st.NumMinersMeetingMinPower > 0 {
		return false, nil
	}

	// else if none do, check whether in MIN_MINER_SIZE_TARG miners
	if st.MinerCount <= ConsensusMinerMinMiners {
		// miner should pass
		return true, nil
	}

	var minerSizes []abi.StoragePower
	var pwr abi.StoragePower
	if err := adt.AsMap(s, st.ClaimedPower).ForEach(&pwr, func(k string) error {
		maddr, err := addr.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		nominalPower, err := st.computeNominalPower(s, maddr, pwr)
		if err != nil {
			return err
		}
		minerSizes = append(minerSizes, nominalPower)
		return nil
	}); err != nil {
		return false, errors.Wrap(err, "failed to iterate power table")
	}

	// get size of MIN_MINER_SIZE_TARGth largest miner
	sort.Slice(minerSizes, func(i, j int) bool { return i > j })
	return minerPower.GreaterThanEqual(minerSizes[ConsensusMinerMinMiners-1]), nil
}

// selectMinersToSurprise implements the PoSt-Surprise sampling algorithm
func (st *StoragePowerActorState) selectMinersToSurprise(s adt.Store, challengeCount int64, randomness abi.Randomness) ([]addr.Address, error) {
	var allMiners []addr.Address
	var pwr abi.StoragePower
	if err := adt.AsMap(s, st.ClaimedPower).ForEach(&pwr, func(k string) error {
		maddr, err := addr.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		nominalPower, err := st.computeNominalPower(s, maddr, pwr)
		if err != nil {
			return err
		}
		if nominalPower.GreaterThan(big.Zero()) {
			allMiners = append(allMiners, maddr)
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to iterate ClaimedPower hamt when selecting miners to surprise")
	}

	selectedMiners := make([]addr.Address, 0)
	for chall := int64(0); chall < challengeCount; chall++ {
		minerIndex := crypto.RandomInt(randomness, chall, st.MinerCount)
		potentialChallengee := allMiners[minerIndex]
		// skip dups
		for addrInArray(potentialChallengee, selectedMiners) {
			minerIndex := crypto.RandomInt(randomness, chall, st.MinerCount)
			potentialChallengee = allMiners[minerIndex]
		}
		selectedMiners = append(selectedMiners, potentialChallengee)
	}

	return selectedMiners, nil
}

func (st *StoragePowerActorState) getMinerPledge(store adt.Store, miner addr.Address) (abi.TokenAmount, error) {
	table := adt.AsBalanceTable(store, st.EscrowTable)
	return table.Get(miner)
}

func (st *StoragePowerActorState) setMinerPledge(store adt.Store, miner addr.Address, amount abi.TokenAmount) error {
	table := adt.AsBalanceTable(store, st.EscrowTable)
	if table.Set(miner, amount) == nil {
		st.EscrowTable = table.Root()
	}
	return nil
}

func (st *StoragePowerActorState) addMinerPledge(store adt.Store, miner addr.Address, amount abi.TokenAmount) error {
	Assert(amount.GreaterThanEqual(big.Zero()))
	table := adt.AsBalanceTable(store, st.EscrowTable)
	if table.Add(miner, amount) == nil {
		st.EscrowTable = table.Root()
	}
	return table.Add(miner, amount)
}

func (st *StoragePowerActorState) subtractMinerPledge(store adt.Store, miner addr.Address, amount abi.TokenAmount,
	balanceFloor abi.TokenAmount) (abi.TokenAmount, error) {
	Assert(amount.GreaterThanEqual(big.Zero()))
	table := adt.AsBalanceTable(store, st.EscrowTable)
	subtracted, err := table.SubtractWithMinimum(miner, amount, balanceFloor)
	if err == nil {
		st.EscrowTable = table.Root()
	}
	return subtracted, err
}

func (st *StoragePowerActorState) addClaimedPowerForSector(s adt.Store, minerAddr addr.Address, weight SectorStorageWeightDesc) error {
	// Note: The following computation does not use any of the dynamic information from CurrIndices();
	// it depends only on weight. This means that the power of a given weight
	// does not vary over time, so we can avoid continually updating it for each sector every epoch.
	//
	// The function is located in the indices module temporarily, until we find a better place for
	// global parameterization functions.
	sectorPower := consensusPowerForWeight(weight)

	currentPower, ok, err := getStoragePower(s, st.ClaimedPower, minerAddr)
	if err != nil {
		return err
	}
	if !ok {
		return errors.Errorf("no power for actor %v", minerAddr)
	}

	if err = st.setClaimedPower(s, minerAddr, big.Add(currentPower, sectorPower)); err != nil {
		return err
	}
	return nil
}

func (st *StoragePowerActorState) deductClaimedPowerForSector(s adt.Store, minerAddr addr.Address, weight SectorStorageWeightDesc) error {
	sectorPower := consensusPowerForWeight(weight)

	currentPower, ok, err := getStoragePower(s, st.ClaimedPower, minerAddr)
	if err != nil {
		return errors.Wrap(err, "failed to get claimed miner power")
	}
	if !ok {
		return errors.Errorf("no claimed power for actor %v", minerAddr)
	}

	if err = st.setClaimedPower(s, minerAddr, big.Sub(currentPower, sectorPower)); err != nil {
		return err
	}
	return nil
}

func (st *StoragePowerActorState) computeNominalPower(s adt.Store, minerAddr addr.Address, claimedPower abi.StoragePower) (abi.StoragePower, error) {
	// Compute nominal power: i.e., the power we infer the miner to have (based on the network's
	// PoSt queries), which may not be the same as the claimed power.
	// Currently, the only reason for these to differ is if the miner is in DetectedFault state
	// from a SurprisePoSt challenge. TODO: hs update this
	nominalPower := claimedPower
	if found, err := st.hasFault(s, minerAddr); err != nil {
		return abi.NewStoragePower(0), err
	} else if found {
		nominalPower = big.Zero()
	}

	return nominalPower, nil
}

func (st *StoragePowerActorState) setClaimedPower(s adt.Store, minerAddr addr.Address, updatedMinerClaimedPower abi.StoragePower) error {
	Assert(updatedMinerClaimedPower.GreaterThanEqual(big.Zero()))
	var err error
	st.ClaimedPower, err = putStoragePower(s, st.ClaimedPower, minerAddr, updatedMinerClaimedPower)
	if err != nil {
		return errors.Wrap(err, "failed to set claimed power while setting claimed power table entry")
	}
	return nil
}

func (st *StoragePowerActorState) hasFault(s adt.Store, a addr.Address) (bool, error) {
	faultyMiners := adt.AsSet(s, st.PoStDetectedFaultMiners)
	found, err := faultyMiners.Has(AddrKey(a))
	if err != nil {
		return false, errors.Wrapf(err, "failed to get detected faults for address %v from set %s", a, st.PoStDetectedFaultMiners)
	}
	return found, nil
}

func (st *StoragePowerActorState) putFault(s adt.Store, a addr.Address) error {
	faultyMiners := adt.AsSet(s, st.PoStDetectedFaultMiners)
	if err := faultyMiners.Put(AddrKey(a)); err != nil {
		return errors.Wrapf(err, "failed to put detected fault for miner %s in set %s", a, st.PoStDetectedFaultMiners)
	}
	st.PoStDetectedFaultMiners = faultyMiners.Root()
	return nil
}

func (st *StoragePowerActorState) deleteFault(s adt.Store, a addr.Address) error {
	faultyMiners := adt.AsSet(s, st.PoStDetectedFaultMiners)
	if err := faultyMiners.Delete(AddrKey(a)); err != nil {
		return errors.Wrapf(err, "failed to delete storage power at address %s from set %s", a, st.PoStDetectedFaultMiners)
	}
	st.PoStDetectedFaultMiners = faultyMiners.Root()
	return nil
}

func (st *StoragePowerActorState) appendCronEvent(store adt.Store, epoch abi.ChainEpoch, event *CronEvent) error {
	mmap := adt.AsMultimap(store, st.CronEventQueue)
	err := mmap.Add(IntKey(epoch), event)
	if err != nil {
		return errors.Wrapf(err, "failed to store cron event at epoch %v for miner %v", epoch, event)
	}
	st.CronEventQueue = mmap.Root()
	return nil
}

func (st *StoragePowerActorState) loadCronEvents(store adt.Store, epoch abi.ChainEpoch) ([]CronEvent, error) {
	mmap := adt.AsMultimap(store, st.CronEventQueue)
	var events []CronEvent
	var ev CronEvent
	err := mmap.ForEach(IntKey(epoch), &ev, func(i int64) error {
		// Ignore events for defunct miners.
		if _, found, err := getStoragePower(store, st.ClaimedPower, ev.MinerAddr); err != nil {
			return errors.Wrapf(err, "failed to find claimed power for %v for cron event", ev.MinerAddr)
		} else if found {
			events = append(events, ev)
		}
		return nil
	})
	return events, err
}

func (st *StoragePowerActorState) clearCronEvents(store adt.Store, epoch abi.ChainEpoch) error {
	mmap := adt.AsMultimap(store, st.CronEventQueue)
	err := mmap.RemoveAll(IntKey(epoch))
	if err != nil {
		return errors.Wrapf(err, "failed to clear cron events")
	}
	st.CronEventQueue = mmap.Root()
	return nil
}

func getStoragePower(s adt.Store, root cid.Cid, a addr.Address) (abi.StoragePower, bool, error) {
	hm := adt.AsMap(s, root)

	var out abi.StoragePower
	found, err := hm.Get(AddrKey(a), &out)
	if err != nil {
		return abi.NewStoragePower(0), false, errors.Wrapf(err, "failed to get storage power for address %v from store %s", a, root)
	}
	if !found {
		return abi.NewStoragePower(0), false, nil
	}
	return out, true, nil
}

func putStoragePower(s adt.Store, root cid.Cid, a addr.Address, pwr abi.StoragePower) (cid.Cid, error) {
	hm := adt.AsMap(s, root)

	if err := hm.Put(AddrKey(a), &pwr); err != nil {
		return cid.Undef, errors.Wrapf(err, "failed to put storage power with address %s power %v in store %s", a, pwr, root)
	}
	return hm.Root(), nil
}

func deleteStoragePower(s adt.Store, root cid.Cid, a addr.Address) (cid.Cid, error) {
	hm := adt.AsMap(s, root)

	if err := hm.Delete(AddrKey(a)); err != nil {
		return cid.Undef, errors.Wrapf(err, "failed to delete storage power at address %s from store %s", a, root)
	}

	return hm.Root(), nil
}

func addrInArray(a addr.Address, list []addr.Address) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
