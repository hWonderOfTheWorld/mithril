package sealevel

import (
	"bytes"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"go.firedancer.io/radiance/pkg/features"
	"go.firedancer.io/radiance/pkg/safemath"
	"k8s.io/klog/v2"
)

const (
	UpgradeableLoaderInstrTypeInitializeBuffer = iota
	UpgradeableLoaderInstrTypeWrite
	UpgradeableLoaderInstrTypeDeployWithMaxDataLen
	UpgradeableLoaderInstrTypeUpgrade
	UpgradeableLoaderInstrTypeSetAuthority
	UpgradeableLoaderInstrTypeClose
	UpgradeableLoaderInstrTypeExtendProgram
	UpgradeableLoaderInstrTypeSetAuthorityChecked
)

const (
	UpgradeableLoaderStateTypeUninitialized = iota
	UpgradeableLoaderStateTypeBuffer
	UpgradeableLoaderStateTypeProgram
	UpgradeableLoaderStateTypeProgramData
)

// instructions
type UpgradeableLoaderInstrWrite struct {
	Offset uint32
	Bytes  []byte
}

type UpgradeableLoaderInstrDeployWithMaxDataLen struct {
	MaxDataLen uint64
}

type UpgradeableLoaderInstrExtendProgram struct {
	AdditionalBytes uint32
}

// upgradeable loader account states
type UpgradeableLoaderStateBuffer struct {
	AuthorityAddress *solana.PublicKey
}

type UpgradeableLoaderStateProgram struct {
	ProgramDataAddress solana.PublicKey
}

type UpgradeableLoaderStateProgramData struct {
	Slot                    uint64
	UpgradeAuthorityAddress *solana.PublicKey
}

type UpgradeableLoaderState struct {
	Type        uint32
	Buffer      UpgradeableLoaderStateBuffer
	Program     UpgradeableLoaderStateProgram
	ProgramData UpgradeableLoaderStateProgramData
}

const upgradeableLoaderSizeOfBufferMetaData = 37
const upgradeableLoaderSizeOfProgram = 36
const upgradeableLoaderSizeOfProgramDataMetaData = 45

func upgradeableLoaderSizeOfProgramData(programLen uint64) uint64 {
	return safemath.SaturatingAddU64(upgradeableLoaderSizeOfProgramDataMetaData, programLen)
}

func upgradeableLoaderSizeOfBuffer(programLen uint64) uint64 {
	return safemath.SaturatingAddU64(upgradeableLoaderSizeOfBufferMetaData, programLen)
}

func (write *UpgradeableLoaderInstrWrite) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	var err error
	write.Offset, err = decoder.ReadUint32(bin.LE)
	if err != nil {
		return err
	}

	write.Bytes, err = decoder.ReadByteSlice()
	return err
}

func (deploy *UpgradeableLoaderInstrDeployWithMaxDataLen) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	var err error
	deploy.MaxDataLen, err = decoder.ReadUint64(bin.LE)
	return err
}

func (extendProgram *UpgradeableLoaderInstrExtendProgram) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	var err error
	extendProgram.AdditionalBytes, err = decoder.ReadUint32(bin.LE)
	return err
}

func (buffer *UpgradeableLoaderStateBuffer) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	hasPubkey, err := decoder.ReadBool()
	if err != nil {
		return err
	}

	if hasPubkey {
		pkBytes, err := decoder.ReadBytes(solana.PublicKeyLength)
		if err != nil {
			return err
		}
		pk := solana.PublicKeyFromBytes(pkBytes)
		buffer.AuthorityAddress = pk.ToPointer()
	}
	return nil
}

func (buffer *UpgradeableLoaderStateBuffer) MarshalWithEncoder(encoder *bin.Encoder) error {
	var err error
	if buffer.AuthorityAddress != nil {
		authAddr := *buffer.AuthorityAddress
		err = encoder.WriteBytes(authAddr.Bytes(), false)
	}
	return err
}

func (program *UpgradeableLoaderStateProgram) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	pkBytes, err := decoder.ReadBytes(solana.PublicKeyLength)
	if err != nil {
		return err
	}
	copy(program.ProgramDataAddress[:], pkBytes)

	return nil
}

func (program *UpgradeableLoaderStateProgram) MarshalWithEncoder(encoder *bin.Encoder) error {
	err := encoder.WriteBytes(program.ProgramDataAddress[:], false)
	return err
}

func (programData *UpgradeableLoaderStateProgramData) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	var err error
	programData.Slot, err = decoder.ReadUint64(bin.LE)
	if err != nil {
		return err
	}

	hasPubkey, err := decoder.ReadBool()
	if err != nil {
		return err
	}

	if hasPubkey {
		pkBytes, err := decoder.ReadBytes(solana.PublicKeyLength)
		if err != nil {
			return err
		}
		pk := solana.PublicKeyFromBytes(pkBytes)
		programData.UpgradeAuthorityAddress = pk.ToPointer()
	}

	return nil
}

func (programData *UpgradeableLoaderStateProgramData) MarshalWithEncoder(encoder *bin.Encoder) error {
	var err error
	err = encoder.WriteUint64(programData.Slot, bin.LE)
	if err != nil {
		return err
	}

	if programData.UpgradeAuthorityAddress != nil {
		upgradeAuthAddr := *programData.UpgradeAuthorityAddress
		err = encoder.WriteBytes(upgradeAuthAddr.Bytes(), false)
	}

	return err
}

func (state *UpgradeableLoaderState) UnmarshalWithDecoder(decoder *bin.Decoder) error {
	var err error

	state.Type, err = decoder.ReadUint32(bin.LE)
	if err != nil {
		return err
	}

	switch state.Type {
	case UpgradeableLoaderStateTypeUninitialized:
		{
			// nothing to deserialize
		}

	case UpgradeableLoaderStateTypeBuffer:
		{
			err = state.Buffer.UnmarshalWithDecoder(decoder)
		}

	case UpgradeableLoaderStateTypeProgram:
		{
			err = state.Program.UnmarshalWithDecoder(decoder)
		}

	case UpgradeableLoaderStateTypeProgramData:
		{
			err = state.ProgramData.UnmarshalWithDecoder(decoder)
		}

	default:
		{
			err = InstrErrInvalidAccountData
		}
	}

	return err
}

func (state *UpgradeableLoaderState) MarshalWithEncoder(encoder *bin.Encoder) error {
	var err error
	switch state.Type {
	case UpgradeableLoaderStateTypeUninitialized:
		{
			// nothing to deserialize
		}

	case UpgradeableLoaderStateTypeBuffer:
		{
			err = state.Buffer.MarshalWithEncoder(encoder)
		}

	case UpgradeableLoaderStateTypeProgram:
		{
			err = state.Program.MarshalWithEncoder(encoder)
		}

	case UpgradeableLoaderStateTypeProgramData:
		{
			err = state.ProgramData.MarshalWithEncoder(encoder)
		}

	default:
		{
			panic("attempting to serialize up invalid upgradeable loader state - programming error")
		}
	}
	return err
}

func unmarshalUpgradeableLoaderState(data []byte) (*UpgradeableLoaderState, error) {
	state := new(UpgradeableLoaderState)
	decoder := bin.NewBinDecoder(data)

	err := state.UnmarshalWithDecoder(decoder)
	if err != nil {
		return nil, InstrErrInvalidAccountData
	} else {
		return state, nil
	}
}

func marshalUpgradeableLoaderState(state *UpgradeableLoaderState) ([]byte, error) {
	buffer := new(bytes.Buffer)
	encoder := bin.NewBinEncoder(buffer)

	err := state.MarshalWithEncoder(encoder)
	if err != nil {
		return nil, err
	} else {
		return buffer.Bytes(), nil
	}
}

func setUpgradeableLoaderAccountState(acct *BorrowedAccount, state *UpgradeableLoaderState, f features.Features) error {
	acctStateBytes, err := marshalUpgradeableLoaderState(state)
	if err != nil {
		return err
	}

	err = acct.SetState(f, acctStateBytes)
	return err
}

func writeProgramData(execCtx *ExecutionCtx, programDataOffset uint64, bytes []byte) error {
	txCtx := execCtx.TransactionContext
	instrCtx, err := txCtx.CurrentInstructionCtx()
	if err != nil {
		return err
	}

	program, err := instrCtx.BorrowInstructionAccount(txCtx, 0)
	if err != nil {
		return err
	}

	writeOffset := safemath.SaturatingAddU64(programDataOffset, uint64(len(bytes)))
	if uint64(len(program.Data())) < writeOffset {
		klog.Infof("write overflow. acct data len = %d, writeOffset = %d", len(program.Data()), writeOffset)
		return InstrErrAccountDataTooSmall
	}

	copy(program.Account.Data[programDataOffset:writeOffset], bytes)
	return nil
}

func BpfLoaderProgramExecute(execCtx *ExecutionCtx) error {
	txCtx := execCtx.TransactionContext
	instrCtx, err := txCtx.CurrentInstructionCtx()
	if err != nil {
		return err
	}
	programAcct, err := instrCtx.BorrowLastProgramAccount(txCtx)
	if err != nil {
		return err
	}

	if programAcct.Owner() == NativeLoaderAddr {
		programId, err := instrCtx.LastProgramKey(txCtx)
		if err != nil {
			return err
		}
		if programId == BpfLoaderUpgradeableAddr {
			err = execCtx.ComputeMeter.Consume(CUUpgradeableLoaderComputeUnits)
			if err != nil {
				return err
			}
			err = ProcessUpgradeableLoaderInstruction(execCtx)
			return err
		} else if programId == BpfLoaderAddr {
			err = execCtx.ComputeMeter.Consume(CUDefaultLoaderComputeUnits)
			if err != nil {
				return err
			}
			return InstrErrUnsupportedProgramId
		} else if programId == BpfLoaderDeprecatedAddr {
			err = execCtx.ComputeMeter.Consume(CUDeprecatedLoaderComputeUnits)
			if err != nil {
				return err
			}
			return InstrErrUnsupportedProgramId
		} else {
			return InstrErrUnsupportedProgramId
		}
	}

	if !programAcct.IsExecutable() {
		return InstrErrUnsupportedProgramId
	}

	// TODO: program execution

	return nil
}

func UpgradeableLoaderInitializeBuffer(execCtx *ExecutionCtx, txCtx *TransactionCtx, instrCtx *InstructionCtx) error {
	err := instrCtx.CheckNumOfInstructionAccounts(2)
	if err != nil {
		return err
	}

	buffer, err := instrCtx.BorrowInstructionAccount(txCtx, 0)
	if err != nil {
		return err
	}

	state, err := unmarshalUpgradeableLoaderState(buffer.Data())
	if err != nil {
		return err
	}

	if state.Type != UpgradeableLoaderStateTypeUninitialized {
		klog.Infof("Buffer account already initialized")
		return InstrErrAccountAlreadyInitialized
	}

	authorityKeyIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(1)
	if err != nil {
		return err
	}

	authorityKey, err := txCtx.KeyOfAccountAtIndex(authorityKeyIdx)
	if err != nil {
		return err
	}

	state.Type = UpgradeableLoaderStateTypeBuffer
	state.Buffer.AuthorityAddress = authorityKey.ToPointer()

	err = setUpgradeableLoaderAccountState(buffer, state, execCtx.GlobalCtx.Features)
	return err
}

func UpgradeableLoaderWrite(execCtx *ExecutionCtx, txCtx *TransactionCtx, instrCtx *InstructionCtx, write UpgradeableLoaderInstrWrite) error {
	err := instrCtx.CheckNumOfInstructionAccounts(2)
	if err != nil {
		return err
	}

	buffer, err := instrCtx.BorrowInstructionAccount(txCtx, 0)
	if err != nil {
		return err
	}

	state, err := unmarshalUpgradeableLoaderState(buffer.Data())
	if err != nil {
		return err
	}

	if state.Type == UpgradeableLoaderStateTypeBuffer {
		if state.Buffer.AuthorityAddress == nil {
			klog.Infof("Buffer is immutable")
			return InstrErrImmutable
		}

		authorityKeyIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(1)
		if err != nil {
			return err
		}
		authorityKey, err := txCtx.KeyOfAccountAtIndex(authorityKeyIdx)
		if err != nil {
			return err
		}
		if *state.Buffer.AuthorityAddress != authorityKey {
			klog.Errorf("Incorrect buffer authority provided")
			return InstrErrIncorrectAuthority
		}

		isSigner, err := instrCtx.IsInstructionAccountSigner(1)
		if err != nil {
			klog.Infof("Buffer authority did not sign")
			return err
		}

		if !isSigner {
			return InstrErrMissingRequiredSignature
		}
	} else {
		klog.Infof("Invalid buffer account")
		return InstrErrInvalidAccountData
	}

	err = writeProgramData(execCtx, upgradeableLoaderSizeOfBufferMetaData+uint64(write.Offset), write.Bytes)
	return err
}

func UpgradeableLoaderDeployWithMaxDataLen(execCtx *ExecutionCtx, txCtx *TransactionCtx, instrCtx *InstructionCtx, deploy UpgradeableLoaderInstrDeployWithMaxDataLen) error {
	err := instrCtx.CheckNumOfInstructionAccounts(4)
	if err != nil {
		return err
	}

	payerKeyIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(0)
	if err != nil {
		return err
	}
	payerKey, err := txCtx.KeyOfAccountAtIndex(payerKeyIdx)
	if err != nil {
		return err
	}

	programDataKeyIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(1)
	if err != nil {
		return err
	}
	programDataKey, err := txCtx.KeyOfAccountAtIndex(programDataKeyIdx)
	if err != nil {
		return err
	}

	rent := ReadRentSysvar(&execCtx.Accounts)
	err = checkAcctForRentSysvar(txCtx, instrCtx, 4)
	if err != nil {
		return err
	}

	clock := ReadClockSysvar(&execCtx.Accounts)
	err = checkAcctForClockSysvar(txCtx, instrCtx, 5)
	if err != nil {
		return err
	}

	err = instrCtx.CheckNumOfInstructionAccounts(8)
	if err != nil {
		return err
	}

	var authorityKey *solana.PublicKey
	authorityIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(1)
	if err == nil {
		k, err := txCtx.KeyOfAccountAtIndex(authorityIdx)
		if err != nil {
			return err
		}
		authorityKey = k.ToPointer()
	}

	// validate program account
	program, err := instrCtx.BorrowInstructionAccount(txCtx, 2)
	if err != nil {
		return err
	}

	programAcctState, err := unmarshalUpgradeableLoaderState(program.Data())
	if err != nil {
		return err
	}

	if programAcctState.Type != UpgradeableLoaderStateTypeUninitialized {
		return InstrErrAccountAlreadyInitialized
	}

	if len(program.Data()) < upgradeableLoaderSizeOfProgram {
		return InstrErrAccountDataTooSmall
	}

	if program.Lamports() < rent.MinimumBalance(uint64(len(program.Data()))) {
		return InstrErrExecutableAccountNotRentExempt
	}

	newProgramId := program.Key()

	// validate buffer account
	buffer, err := instrCtx.BorrowInstructionAccount(txCtx, 3)
	if err != nil {
		return err
	}

	bufferAcctState, err := unmarshalUpgradeableLoaderState(program.Data())
	if err != nil {
		return err
	}

	if bufferAcctState.Type != UpgradeableLoaderStateTypeBuffer {
		return InstrErrInvalidArgument
	}

	if bufferAcctState.Buffer.AuthorityAddress != nil && authorityKey != nil &&
		*bufferAcctState.Buffer.AuthorityAddress != *authorityKey {
		return InstrErrIncorrectAuthority
	}

	isSigner, err := instrCtx.IsInstructionAccountSigner(7)
	if err != nil {
		return err
	}
	if !isSigner {
		return InstrErrMissingRequiredSignature
	}

	bufferKey := buffer.Key()
	bufferDataOffset := uint64(upgradeableLoaderSizeOfBufferMetaData)
	bufferDataLen := safemath.SaturatingSubU64(uint64(len(buffer.Data())), bufferDataOffset)
	programDataDataOffset := uint64(upgradeableLoaderSizeOfProgramDataMetaData)
	programDataLen := upgradeableLoaderSizeOfProgramData(deploy.MaxDataLen)

	if uint64(len(buffer.Account.Data)) < upgradeableLoaderSizeOfBufferMetaData || bufferDataLen == 0 {
		return InstrErrInvalidAccountData
	}

	if deploy.MaxDataLen < bufferDataLen {
		return InstrErrAccountDataTooSmall
	}

	if programDataLen > MaxPermittedDataLength {
		return InstrErrInvalidArgument
	}

	seed := make([][]byte, 1)
	seed[0] = make([]byte, solana.PublicKeyLength)
	copy(seed[0], newProgramId[:])

	programId, err := instrCtx.LastProgramKey(txCtx)
	if err != nil {
		return err
	}

	derivedAddr, bumpSeed, _ := solana.FindProgramAddress(seed, programId)
	if derivedAddr != programDataKey {
		return InstrErrInvalidArgument
	}

	payer, err := instrCtx.BorrowInstructionAccount(txCtx, 0)
	if err != nil {
		return err
	}
	payer.CheckedAddLamports(buffer.Lamports(), execCtx.GlobalCtx.Features)
	buffer.SetLamports(0, execCtx.GlobalCtx.Features)

	//ownerId := programId

	var lamports uint64
	minBalance := rent.MinimumBalance(programDataLen)
	if minBalance > 1 {
		lamports = minBalance
	} else {
		lamports = 1
	}
	createAcctInstr := newCreateAccountInstruction(payerKey, programDataKey, lamports, programDataLen, programId)
	createAcctInstr.Accounts = append(createAcctInstr.Accounts, AccountMeta{Pubkey: bufferKey, IsSigner: false, IsWritable: true})

	callerProgramId, err := instrCtx.LastProgramKey(txCtx)
	if err != nil {
		return err
	}

	var seeds [][]byte
	seeds = append(seeds, newProgramId[:])
	seeds = append(seeds, []byte{bumpSeed})

	signer, err := solana.CreateProgramAddress(seeds, callerProgramId)
	if err != nil {
		return err
	}

	var signers []solana.PublicKey
	signers = append(signers, signer)

	err = execCtx.NativeInvoke(*createAcctInstr, signers)
	if err != nil {
		return err
	}

	// TODO: deploy_program!

	programData, err := instrCtx.BorrowInstructionAccount(txCtx, 1)
	if err != nil {
		return err
	}

	programDataState := &UpgradeableLoaderState{Type: UpgradeableLoaderStateTypeProgramData,
		ProgramData: UpgradeableLoaderStateProgramData{Slot: clock.Slot, UpgradeAuthorityAddress: authorityKey}}

	err = setUpgradeableLoaderAccountState(programData, programDataState, execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	dstEnd := safemath.SaturatingAddU64(programDataDataOffset, bufferDataLen)
	if uint64(len(programData.Data())) < dstEnd {
		return InstrErrAccountDataTooSmall
	}
	if uint64(len(programData.Data())) < bufferDataOffset {
		return InstrErrAccountDataTooSmall
	}

	dstSlice := programData.Account.Data[programDataDataOffset:dstEnd]
	srcSlice := buffer.Account.Data[bufferDataOffset:]
	copy(dstSlice, srcSlice)

	err = buffer.SetDataLength(upgradeableLoaderSizeOfBuffer(0), execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	programState := &UpgradeableLoaderState{Type: UpgradeableLoaderStateTypeProgram,
		Program: UpgradeableLoaderStateProgram{ProgramDataAddress: programDataKey}}
	err = setUpgradeableLoaderAccountState(program, programState, execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	if !execCtx.GlobalCtx.Features.IsActive(features.DeprecateExecutableMetaUpdateInBpfLoader) {
		err = program.SetExecutable(true)
		if err != nil {
			return err
		}
	}

	return nil
}

func UpgradeableLoaderUpgrade(execCtx *ExecutionCtx, txCtx *TransactionCtx, instrCtx *InstructionCtx) error {
	err := instrCtx.CheckNumOfInstructionAccounts(3)
	if err != nil {
		return err
	}

	programDataKeyIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(0)
	if err != nil {
		return err
	}
	programDataKey, err := txCtx.KeyOfAccountAtIndex(programDataKeyIdx)
	if err != nil {
		return err
	}

	rent := ReadRentSysvar(&execCtx.Accounts)
	err = checkAcctForRentSysvar(txCtx, instrCtx, 4)
	if err != nil {
		return err
	}

	clock := ReadClockSysvar(&execCtx.Accounts)
	err = checkAcctForClockSysvar(txCtx, instrCtx, 5)
	if err != nil {
		return err
	}

	err = instrCtx.CheckNumOfInstructionAccounts(7)
	if err != nil {
		return err
	}

	authorityKeyIdx, err := instrCtx.IndexOfInstructionAccountInTransaction(6)
	if err != nil {
		return err
	}
	authorityKey, err := txCtx.KeyOfAccountAtIndex(authorityKeyIdx)
	if err != nil {
		return err
	}

	program, err := instrCtx.BorrowInstructionAccount(txCtx, 1)
	if err != nil {
		return err
	}

	if !program.IsExecutable() {
		return InstrErrAccountNotExecutable
	}

	if !program.IsWritable() {
		return InstrErrInvalidArgument
	}

	programId, err := instrCtx.LastProgramKey(txCtx)
	if err != nil {
		return err
	}
	if program.Owner() != programId {
		return InstrErrIncorrectProgramId
	}

	programState, err := unmarshalUpgradeableLoaderState(program.Data())
	if err != nil {
		return err
	}

	if programState.Type == UpgradeableLoaderStateTypeProgram {
		if programState.Program.ProgramDataAddress != programDataKey {
			return InstrErrInvalidArgument
		}
	} else {
		return InstrErrInvalidAccountData
	}

	buffer, err := instrCtx.BorrowInstructionAccount(txCtx, 2)
	if err != nil {
		return err
	}

	bufferState, err := unmarshalUpgradeableLoaderState(buffer.Data())
	if bufferState.Type == UpgradeableLoaderStateTypeBuffer {
		if bufferState.Buffer.AuthorityAddress == nil || *bufferState.Buffer.AuthorityAddress != authorityKey {
			return InstrErrIncorrectAuthority
		}
		isSigner, err := instrCtx.IsInstructionAccountSigner(6)
		if err != nil {
			return err
		}
		if !isSigner {
			return InstrErrMissingRequiredSignature
		}
	} else {
		return InstrErrInvalidArgument
	}

	bufferLamports := buffer.Lamports()
	bufferDataOffset := uint64(upgradeableLoaderSizeOfBufferMetaData)
	bufferDataLen := safemath.SaturatingSubU64(uint64(len(buffer.Data())), bufferDataOffset)
	if len(buffer.Data()) < upgradeableLoaderSizeOfBufferMetaData || bufferDataLen == 0 {
		return InstrErrInvalidAccountData
	}

	programData, err := instrCtx.BorrowInstructionAccount(txCtx, 0)
	if err != nil {
		return err
	}

	var programDataBalanceRequired uint64
	minBalance := rent.MinimumBalance(uint64(len(programData.Data())))
	if minBalance > 1 {
		programDataBalanceRequired = minBalance
	} else {
		programDataBalanceRequired = 1
	}

	if len(programData.Data()) < int(upgradeableLoaderSizeOfProgramData(bufferDataLen)) {
		return InstrErrAccountDataTooSmall
	}

	if safemath.SaturatingAddU64(programData.Lamports(), bufferLamports) < programDataBalanceRequired {
		return InstrErrInsufficientFunds
	}

	programDataState, err := unmarshalUpgradeableLoaderState(programData.Data())
	if err != nil {
		return err
	}

	if programDataState.Type == UpgradeableLoaderStateTypeProgramData {
		if clock.Slot == programDataState.ProgramData.Slot {
			return InstrErrInvalidArgument
		}
		if programDataState.ProgramData.UpgradeAuthorityAddress == nil {
			return InstrErrImmutable
		}
		if *programDataState.ProgramData.UpgradeAuthorityAddress != authorityKey {
			return InstrErrIncorrectAuthority
		}
		isSigner, err := instrCtx.IsInstructionAccountSigner(6)
		if err != nil {
			return err
		}
		if !isSigner {
			return InstrErrMissingRequiredSignature
		}
	} else {
		return InstrErrInvalidAccountData
	}

	// deploy_program! ...

	programDataNewState := &UpgradeableLoaderState{ProgramData: UpgradeableLoaderStateProgramData{Slot: clock.Slot, UpgradeAuthorityAddress: &authorityKey}}
	err = setUpgradeableLoaderAccountState(programData, programDataNewState, execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	programDataDataOffset := uint64(upgradeableLoaderSizeOfProgramDataMetaData)
	dstEnd := safemath.SaturatingAddU64(programDataDataOffset, bufferDataLen)
	if uint64(len(programData.Data())) < dstEnd {
		return InstrErrAccountDataTooSmall
	}
	if uint64(len(programData.Data())) < bufferDataOffset {
		return InstrErrAccountDataTooSmall
	}

	dstSlice := programData.Account.Data[programDataDataOffset:dstEnd]
	srcSlice := buffer.Account.Data[bufferDataOffset:]
	copy(dstSlice, srcSlice)

	programDataFillSlice := programData.Account.Data[dstEnd:]
	for i := range programDataFillSlice {
		programDataFillSlice[i] = 0
	}

	spill, err := instrCtx.BorrowInstructionAccount(txCtx, 3)
	if err != nil {
		return err
	}

	spillLamports := safemath.SaturatingSubU64(safemath.SaturatingAddU64(programData.Lamports(), bufferLamports), programDataBalanceRequired)
	err = spill.CheckedAddLamports(spillLamports, execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	err = buffer.SetLamports(0, execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}
	err = programData.SetLamports(programDataBalanceRequired, execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	err = buffer.SetDataLength(upgradeableLoaderSizeOfBuffer(0), execCtx.GlobalCtx.Features)
	if err != nil {
		return err
	}

	klog.Infof("upgraded program %s", program.Key())
	return nil
}

func ProcessUpgradeableLoaderInstruction(execCtx *ExecutionCtx) error {
	txCtx := execCtx.TransactionContext
	instrCtx, err := txCtx.CurrentInstructionCtx()
	if err != nil {
		return err
	}

	instrData := instrCtx.Data
	decoder := bin.NewBinDecoder(instrData)

	instrType, err := decoder.ReadUint32(bin.LE)
	if err != nil {
		return InstrErrInvalidInstructionData
	}

	switch instrType {
	case UpgradeableLoaderInstrTypeInitializeBuffer:
		{
			err = UpgradeableLoaderInitializeBuffer(execCtx, txCtx, instrCtx)
		}

	case UpgradeableLoaderInstrTypeWrite:
		{
			var write UpgradeableLoaderInstrWrite
			err = write.UnmarshalWithDecoder(decoder)
			if err != nil {
				return InstrErrInvalidInstructionData
			}

			err = UpgradeableLoaderWrite(execCtx, txCtx, instrCtx, write)
		}

	case UpgradeableLoaderInstrTypeDeployWithMaxDataLen:
		{
			var deploy UpgradeableLoaderInstrDeployWithMaxDataLen
			err = deploy.UnmarshalWithDecoder(decoder)
			if err != nil {
				return InstrErrInvalidInstructionData
			}

			err = UpgradeableLoaderDeployWithMaxDataLen(execCtx, txCtx, instrCtx, deploy)
		}

	case UpgradeableLoaderInstrTypeUpgrade:
		{
			err = UpgradeableLoaderUpgrade(execCtx, txCtx, instrCtx)
		}
	default:
		{
			err = InstrErrInvalidInstructionData
		}
	}

	return err
}
