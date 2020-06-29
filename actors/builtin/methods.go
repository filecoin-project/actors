package builtin

import (
	abi "github.com/filecoin-project/specs-actors/actors/abi"
)

const (
	MethodSend        = abi.MethodNum(0)
	MethodConstructor = abi.MethodNum(1)

	// TODO fin: remove this once canonical method numbers are finalized
	MethodPlaceholder = abi.MethodNum(1 << 30)
)

var MethodsAccount = struct {
	Constructor   abi.MethodNum
	PubkeyAddress abi.MethodNum
}{MethodConstructor, 2}

var MethodsInit = struct {
	Constructor abi.MethodNum
	Exec        abi.MethodNum
}{MethodConstructor, 2}

var MethodsCron = struct {
	Constructor abi.MethodNum
	EpochTick   abi.MethodNum
}{MethodConstructor, 2}

var MethodsReward = struct {
	Constructor        abi.MethodNum
	AwardBlockReward   abi.MethodNum
	WithdrawReward     abi.MethodNum
	LastPerEpochReward abi.MethodNum
	UpdateNetworkKPI   abi.MethodNum
}{MethodConstructor, 2, 3, 4, 5}

var MethodsMultisig = struct {
	Constructor                 abi.MethodNum
	Propose                     abi.MethodNum
	Approve                     abi.MethodNum
	Cancel                      abi.MethodNum
	ClearCompleted              abi.MethodNum
	AddSigner                   abi.MethodNum
	RemoveSigner                abi.MethodNum
	SwapSigner                  abi.MethodNum
	ChangeNumApprovalsThreshold abi.MethodNum
}{MethodConstructor, 2, 3, 4, 5, 6, 7, 8, 9}

var MethodsPaych = struct {
	Constructor        abi.MethodNum
	UpdateChannelState abi.MethodNum
	Settle             abi.MethodNum
	Collect            abi.MethodNum
}{MethodConstructor, 2, 3, 4}

var MethodsMarket = struct {
	Constructor                    abi.MethodNum
	AddBalance                     abi.MethodNum
	WithdrawBalance                abi.MethodNum
	HandleExpiredDeals             abi.MethodNum
	PublishStorageDeals            abi.MethodNum
	VerifyDealsOnSectorProveCommit abi.MethodNum
	OnMinerSectorsTerminate        abi.MethodNum
	ComputeDataCommitment          abi.MethodNum
	GetWeightForDealSet            abi.MethodNum
}{MethodConstructor, 2, 3, 4, 5, 6, 7, 8, 9}

var MethodsPower = struct {
	Constructor                          abi.MethodNum
	AddBalance                           abi.MethodNum
	WithdrawBalance                      abi.MethodNum
	CreateMiner                          abi.MethodNum
	DeleteMiner                          abi.MethodNum
	OnSectorProveCommit                  abi.MethodNum
	OnSectorTerminate                    abi.MethodNum
	OnSectorTemporaryFaultEffectiveBegin abi.MethodNum
	OnSectorTemporaryFaultEffectiveEnd   abi.MethodNum
	OnSectorModifyWeightDesc             abi.MethodNum
	OnMinerWindowedPoStSuccess           abi.MethodNum
	OnMinerWindowedPoStFailure           abi.MethodNum
	EnrollCronEvent                      abi.MethodNum
	ReportConsensusFault                 abi.MethodNum
	OnEpochTickEnd                       abi.MethodNum
}{MethodConstructor, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

var MethodsMiner = struct {
	Constructor            abi.MethodNum
	ControlAddresses       abi.MethodNum
	ChangeWorkerAddress    abi.MethodNum
	ChangePeerID           abi.MethodNum
	SubmitWindowedPoSt     abi.MethodNum
	OnDeleteMiner          abi.MethodNum
	PreCommitSector        abi.MethodNum
	ProveCommitSector      abi.MethodNum
	ExtendSectorExpiration abi.MethodNum
	TerminateSectors       abi.MethodNum
	DeclareTemporaryFaults abi.MethodNum
	OnDeferredCronEvent    abi.MethodNum
	CheckSectorProven      abi.MethodNum
}{MethodConstructor, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
