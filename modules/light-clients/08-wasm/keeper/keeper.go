package keeper

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	wasmvm "github.com/CosmWasm/wasmvm"

	storetypes "cosmossdk.io/core/store"
	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/ibc-go/modules/light-clients/08-wasm/internal/ibcwasm"
	"github.com/cosmos/ibc-go/modules/light-clients/08-wasm/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
)

// Keeper defines the 08-wasm keeper
type Keeper struct {
	// implements gRPC QueryServer interface
	types.QueryServer

	cdc    codec.BinaryCodec
	wasmVM ibcwasm.WasmEngine

	clientKeeper types.ClientKeeper

	authority string
}

// NewKeeperWithVM creates a new Keeper instance with the provided Wasm VM.
// This constructor function is meant to be used when the chain uses x/wasm
// and the same Wasm VM instance should be shared with it.
func NewKeeperWithVM(
	cdc codec.BinaryCodec,
	storeService storetypes.KVStoreService,
	clientKeeper types.ClientKeeper,
	authority string,
	vm ibcwasm.WasmEngine,
) Keeper {
	if clientKeeper == nil {
		panic(errors.New("client keeper must be not nil"))
	}

	if vm == nil {
		panic(errors.New("wasm VM must be not nil"))
	}

	if storeService == nil {
		panic(errors.New("store service must be not nil"))
	}

	if strings.TrimSpace(authority) == "" {
		panic(errors.New("authority must be non-empty"))
	}

	ibcwasm.SetVM(vm)
	ibcwasm.SetupWasmStoreService(storeService)

	return Keeper{
		cdc:          cdc,
		wasmVM:       vm,
		clientKeeper: clientKeeper,
		authority:    authority,
	}
}

// NewKeeperWithConfig creates a new Keeper instance with the provided Wasm configuration.
// This constructor function is meant to be used when the chain does not use x/wasm
// and a Wasm VM needs to be instantiated using the provided parameters.
func NewKeeperWithConfig(
	cdc codec.BinaryCodec,
	storeService storetypes.KVStoreService,
	clientKeeper types.ClientKeeper,
	authority string,
	wasmConfig types.WasmConfig,
) Keeper {
	vm, err := wasmvm.NewVM(wasmConfig.DataDir, wasmConfig.SupportedCapabilities, types.ContractMemoryLimit, wasmConfig.ContractDebugMode, types.MemoryCacheSize)
	if err != nil {
		panic(fmt.Errorf("failed to instantiate new Wasm VM instance: %v", err))
	}

	return NewKeeperWithVM(cdc, storeService, clientKeeper, authority, vm)
}

// GetAuthority returns the 08-wasm module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

func generateWasmCodeHash(code []byte) []byte {
	hash := sha256.Sum256(code)
	return hash[:]
}

func (k Keeper) storeWasmCode(ctx sdk.Context, code []byte) ([]byte, error) {
	var err error
	if types.IsGzip(code) {
		ctx.GasMeter().ConsumeGas(types.VMGasRegister.UncompressCosts(len(code)), "Uncompress gzip bytecode")
		code, err = types.Uncompress(code, types.MaxWasmByteSize())
		if err != nil {
			return nil, errorsmod.Wrap(err, "failed to store contract")
		}
	}

	// Check to see if store already has codeHash.
	codeHash := generateWasmCodeHash(code)
	if types.HasCodeHash(ctx, codeHash) {
		return nil, types.ErrWasmCodeExists
	}

	// run the code through the wasm light client validation process
	if err := types.ValidateWasmCode(code); err != nil {
		return nil, errorsmod.Wrap(err, "wasm bytecode validation failed")
	}

	// create the code in the vm
	ctx.GasMeter().ConsumeGas(types.VMGasRegister.CompileCosts(len(code)), "Compiling wasm bytecode")
	vmCodeHash, err := k.wasmVM.StoreCode(code)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to store contract")
	}

	// SANITY: We've checked our store, additional safety check to assert that the code hash returned by WasmVM equals code hash generated by us.
	if !bytes.Equal(vmCodeHash, codeHash) {
		return nil, errorsmod.Wrapf(types.ErrInvalidCodeHash, "expected %s, got %s", hex.EncodeToString(codeHash), hex.EncodeToString(vmCodeHash))
	}

	// pin the code to the vm in-memory cache
	if err := k.wasmVM.Pin(vmCodeHash); err != nil {
		return nil, errorsmod.Wrapf(err, "failed to pin contract with code hash (%s) to vm cache", hex.EncodeToString(vmCodeHash))
	}

	// store the code hash
	err = ibcwasm.CodeHashes.Set(ctx, codeHash)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to store code hash")
	}

	return codeHash, nil
}

func (k Keeper) migrateContractCode(ctx sdk.Context, clientID string, newCodeHash, migrateMsg []byte) error {
	wasmClientState, err := k.GetWasmClientState(ctx, clientID)
	if err != nil {
		return errorsmod.Wrap(err, "failed to retrieve wasm client state")
	}
	oldCodeHash := wasmClientState.CodeHash

	clientStore := k.clientKeeper.ClientStore(ctx, clientID)

	err = wasmClientState.MigrateContract(ctx, k.cdc, clientStore, clientID, newCodeHash, migrateMsg)
	if err != nil {
		return errorsmod.Wrap(err, "contract migration failed")
	}

	// client state may be updated by the contract migration
	wasmClientState, err = k.GetWasmClientState(ctx, clientID)
	if err != nil {
		// note that this also ensures that the updated client state is
		// still a wasm client state
		return errorsmod.Wrap(err, "failed to retrieve the updated wasm client state")
	}

	// update the client state code hash before persisting it
	wasmClientState.CodeHash = newCodeHash

	k.clientKeeper.SetClientState(ctx, clientID, wasmClientState)

	emitMigrateContractEvent(ctx, clientID, oldCodeHash, newCodeHash)

	return nil
}

// GetWasmClientState returns the 08-wasm client state for the given client identifier.
func (k Keeper) GetWasmClientState(ctx sdk.Context, clientID string) (*types.ClientState, error) {
	clientState, found := k.clientKeeper.GetClientState(ctx, clientID)
	if !found {
		return nil, errorsmod.Wrapf(clienttypes.ErrClientTypeNotFound, "clientID %s", clientID)
	}

	wasmClientState, ok := clientState.(*types.ClientState)
	if !ok {
		return nil, errorsmod.Wrapf(clienttypes.ErrInvalidClient, "expected type %T, got %T", (*types.ClientState)(nil), wasmClientState)
	}

	return wasmClientState, nil
}