package types

import (
	"reflect"

	"github.com/cosmos/cosmos-sdk/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"

	clienttypes "github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/v3/modules/core/23-commitment/types"
	host "github.com/cosmos/ibc-go/v3/modules/core/24-host"
	"github.com/cosmos/ibc-go/v3/modules/core/exported"
)

var _ exported.ClientState = (*ClientState)(nil)

// NewClientState creates a new ClientState instance.
func NewClientState(latestSequence uint64, consensusState *ConsensusState, allowUpdateAfterProposal bool) *ClientState {
	return &ClientState{
		Sequence:                 latestSequence,
		IsFrozen:                 false,
		ConsensusState:           consensusState,
		AllowUpdateAfterProposal: allowUpdateAfterProposal,
	}
}

// ClientType is Solo Machine.
func (cs ClientState) ClientType() string {
	return exported.Solomachine
}

// GetLatestHeight returns the latest sequence number.
// Return exported.Height to satisfy ClientState interface
// Revision number is always 0 for a solo-machine.
func (cs ClientState) GetLatestHeight() exported.Height {
	return clienttypes.NewHeight(0, cs.Sequence)
}

// GetTimestampAtHeight returns the timestamp in nanoseconds of the consensus state at the given height.
func (cs ClientState) GetTimestampAtHeight(
	_ sdk.Context,
	clientStore sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
) (uint64, error) {
	if !cs.GetLatestHeight().EQ(height) {
		return 0, sdkerrors.Wrapf(ErrInvalidSequence, "not latest height (%s)", height)
	}
	return cs.ConsensusState.Timestamp, nil
}

// Status returns the status of the solo machine client.
// The client may be:
// - Active: if frozen sequence is 0
// - Frozen: otherwise solo machine is frozen
func (cs ClientState) Status(_ sdk.Context, _ sdk.KVStore, _ codec.BinaryCodec) exported.Status {
	if cs.IsFrozen {
		return exported.Frozen
	}

	return exported.Active
}

// Validate performs basic validation of the client state fields.
func (cs ClientState) Validate() error {
	if cs.Sequence == 0 {
		return sdkerrors.Wrap(clienttypes.ErrInvalidClient, "sequence cannot be 0")
	}
	if cs.ConsensusState == nil {
		return sdkerrors.Wrap(clienttypes.ErrInvalidConsensus, "consensus state cannot be nil")
	}
	return cs.ConsensusState.ValidateBasic()
}

// ZeroCustomFields returns solomachine client state with client-specific fields FrozenSequence,
// and AllowUpdateAfterProposal zeroed out
func (cs ClientState) ZeroCustomFields() exported.ClientState {
	return NewClientState(
		cs.Sequence, cs.ConsensusState, false,
	)
}

// Initialize will check that initial consensus state is equal to the latest consensus state of the initial client.
func (cs ClientState) Initialize(_ sdk.Context, _ codec.BinaryCodec, _ sdk.KVStore, consState exported.ConsensusState) error {
	if !reflect.DeepEqual(cs.ConsensusState, consState) {
		return sdkerrors.Wrapf(clienttypes.ErrInvalidConsensus, "consensus state in initial client does not equal initial consensus state. expected: %s, got: %s",
			cs.ConsensusState, consState)
	}
	return nil
}

// ExportMetadata is a no-op since solomachine does not store any metadata in client store
func (cs ClientState) ExportMetadata(_ sdk.KVStore) []exported.GenesisMetadata {
	return nil
}

// VerifyUpgradeAndUpdateState returns an error since solomachine client does not support upgrades
func (cs ClientState) VerifyUpgradeAndUpdateState(
	_ sdk.Context, _ codec.BinaryCodec, _ sdk.KVStore,
	_ exported.ClientState, _ exported.ConsensusState, _, _ []byte,
) error {
	return sdkerrors.Wrap(clienttypes.ErrInvalidUpgradeClient, "cannot upgrade solomachine client")
}

// VerifyClientState verifies a proof of the client state of the running chain
// stored on the solo machine.
func (cs *ClientState) VerifyClientState(
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	prefix exported.Prefix,
	counterpartyClientIdentifier string,
	proof []byte,
	clientState exported.ClientState,
) error {
	// NOTE: the proof height sequence is incremented by one due to the connection handshake verification ordering
	height = clienttypes.NewHeight(height.GetRevisionNumber(), height.GetRevisionHeight()+1)

	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	clientPrefixedPath := commitmenttypes.NewMerklePath(host.FullClientStatePath(counterpartyClientIdentifier))
	path, err := commitmenttypes.ApplyPrefix(prefix, clientPrefixedPath)
	if err != nil {
		return err
	}

	signBz, err := ClientStateSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, clientState)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyClientConsensusState verifies a proof of the consensus state of the
// running chain stored on the solo machine.
func (cs *ClientState) VerifyClientConsensusState(
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	counterpartyClientIdentifier string,
	consensusHeight exported.Height,
	prefix exported.Prefix,
	proof []byte,
	consensusState exported.ConsensusState,
) error {
	// NOTE: the proof height sequence is incremented by two due to the connection handshake verification ordering
	height = clienttypes.NewHeight(height.GetRevisionNumber(), height.GetRevisionHeight()+2)

	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	clientPrefixedPath := commitmenttypes.NewMerklePath(host.FullConsensusStatePath(counterpartyClientIdentifier, consensusHeight))
	path, err := commitmenttypes.ApplyPrefix(prefix, clientPrefixedPath)
	if err != nil {
		return err
	}

	signBz, err := ConsensusStateSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, consensusState)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyConnectionState verifies a proof of the connection state of the
// specified connection end stored on the target machine.
func (cs *ClientState) VerifyConnectionState(
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	prefix exported.Prefix,
	proof []byte,
	connectionID string,
	connectionEnd exported.ConnectionI,
) error {
	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	connectionPath := commitmenttypes.NewMerklePath(host.ConnectionPath(connectionID))
	path, err := commitmenttypes.ApplyPrefix(prefix, connectionPath)
	if err != nil {
		return err
	}

	signBz, err := ConnectionStateSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, connectionEnd)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyChannelState verifies a proof of the channel state of the specified
// channel end, under the specified port, stored on the target machine.
func (cs *ClientState) VerifyChannelState(
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	prefix exported.Prefix,
	proof []byte,
	portID,
	channelID string,
	channel exported.ChannelI,
) error {
	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	channelPath := commitmenttypes.NewMerklePath(host.ChannelPath(portID, channelID))
	path, err := commitmenttypes.ApplyPrefix(prefix, channelPath)
	if err != nil {
		return err
	}

	signBz, err := ChannelStateSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, channel)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyPacketCommitment verifies a proof of an outgoing packet commitment at
// the specified port, specified channel, and specified sequence.
func (cs *ClientState) VerifyPacketCommitment(
	ctx sdk.Context,
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	_ uint64,
	_ uint64,
	prefix exported.Prefix,
	proof []byte,
	portID,
	channelID string,
	packetSequence uint64,
	commitmentBytes []byte,
) error {
	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	commitmentPath := commitmenttypes.NewMerklePath(host.PacketCommitmentPath(portID, channelID, packetSequence))
	path, err := commitmenttypes.ApplyPrefix(prefix, commitmentPath)
	if err != nil {
		return err
	}

	signBz, err := PacketCommitmentSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, commitmentBytes)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyPacketAcknowledgement verifies a proof of an incoming packet
// acknowledgement at the specified port, specified channel, and specified sequence.
func (cs *ClientState) VerifyPacketAcknowledgement(
	ctx sdk.Context,
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	_ uint64,
	_ uint64,
	prefix exported.Prefix,
	proof []byte,
	portID,
	channelID string,
	packetSequence uint64,
	acknowledgement []byte,
) error {
	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	ackPath := commitmenttypes.NewMerklePath(host.PacketAcknowledgementPath(portID, channelID, packetSequence))
	path, err := commitmenttypes.ApplyPrefix(prefix, ackPath)
	if err != nil {
		return err
	}

	signBz, err := PacketAcknowledgementSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, acknowledgement)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyPacketReceiptAbsence verifies a proof of the absence of an
// incoming packet receipt at the specified port, specified channel, and
// specified sequence.
func (cs *ClientState) VerifyPacketReceiptAbsence(
	ctx sdk.Context,
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	_ uint64,
	_ uint64,
	prefix exported.Prefix,
	proof []byte,
	portID,
	channelID string,
	packetSequence uint64,
) error {
	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	receiptPath := commitmenttypes.NewMerklePath(host.PacketReceiptPath(portID, channelID, packetSequence))
	path, err := commitmenttypes.ApplyPrefix(prefix, receiptPath)
	if err != nil {
		return err
	}

	signBz, err := PacketReceiptAbsenceSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyNextSequenceRecv verifies a proof of the next sequence number to be
// received of the specified channel at the specified port.
func (cs *ClientState) VerifyNextSequenceRecv(
	ctx sdk.Context,
	store sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	_ uint64,
	_ uint64,
	prefix exported.Prefix,
	proof []byte,
	portID,
	channelID string,
	nextSequenceRecv uint64,
) error {
	publicKey, sigData, timestamp, sequence, err := produceVerificationArgs(cdc, cs, height, prefix, proof)
	if err != nil {
		return err
	}

	nextSequenceRecvPath := commitmenttypes.NewMerklePath(host.NextSequenceRecvPath(portID, channelID))
	path, err := commitmenttypes.ApplyPrefix(prefix, nextSequenceRecvPath)
	if err != nil {
		return err
	}

	signBz, err := NextSequenceRecvSignBytes(cdc, sequence, timestamp, cs.ConsensusState.Diversifier, path, nextSequenceRecv)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(store, cdc, cs)
	return nil
}

// VerifyMembership is a generic proof verification method which verifies a proof of the existence of a value at a given CommitmentPath at the specified height.
// The caller is expected to construct the full CommitmentPath from a CommitmentPrefix and a standardized path (as defined in ICS 24).
func (cs *ClientState) VerifyMembership(
	ctx sdk.Context,
	clientStore sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	delayTimePeriod uint64,
	delayBlockPeriod uint64,
	proof []byte,
	path []byte,
	value []byte,
) error {
	// TODO: Implement 06-solomachine VerifyMembership
	if revision := height.GetRevisionNumber(); revision != 0 {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidHeight, "revision must be 0 for solomachine, got revision-number: %d", revision)
	}

	// sequence is encoded in the revision height of height struct
	sequence := height.GetRevisionHeight()
	latestSequence := cs.GetLatestHeight().GetRevisionHeight()
	if latestSequence != sequence {
		return sdkerrors.Wrapf(
			sdkerrors.ErrInvalidHeight,
			"client state sequence != proof sequence (%d != %d)", latestSequence, sequence,
		)
	}

	var merklePath commitmenttypes.MerklePath
	if err := cdc.Unmarshal(path, &merklePath); err != nil {
		return sdkerrors.Wrap(commitmenttypes.ErrInvalidProof, "failed to unmarshal path into ICS 23 commitment merkle path")
	}

	var timestampedSigData TimestampedSignatureData
	if err := cdc.Unmarshal(proof, &timestampedSigData); err != nil {
		return sdkerrors.Wrapf(err, "failed to unmarshal proof into type %T", timestampedSigData)
	}

	timestamp := timestampedSigData.Timestamp

	if len(timestampedSigData.SignatureData) == 0 {
		return sdkerrors.Wrap(ErrInvalidProof, "signature data cannot be empty")
	}

	sigData, err := UnmarshalSignatureData(cdc, timestampedSigData.SignatureData)
	if err != nil {
		return err
	}

	if cs.ConsensusState == nil {
		return sdkerrors.Wrap(clienttypes.ErrInvalidConsensus, "consensus state cannot be empty")
	}

	if cs.ConsensusState.GetTimestamp() > timestamp {
		return sdkerrors.Wrapf(ErrInvalidProof, "the consensus state timestamp is greater than the signature timestamp (%d >= %d)", cs.ConsensusState.GetTimestamp(), timestamp)
	}

	publicKey, err := cs.ConsensusState.GetPubKey()
	if err != nil {
		return err
	}

	signBytes := &SignBytesV2{
		Sequence:    sequence,
		Timestamp:   timestamp,
		Diversifier: cs.ConsensusState.Diversifier,
		Path:        []byte(merklePath.String()),
		Data:        value,
	}

	signBz, err := cdc.Marshal(signBytes)
	if err != nil {
		return err
	}

	if err := VerifySignature(publicKey, signBz, sigData); err != nil {
		return err
	}

	cs.Sequence++
	cs.ConsensusState.Timestamp = timestamp
	setClientState(clientStore, cdc, cs)

	return nil
}

// VerifyNonMembership is a generic proof verification method which verifies the absense of a given CommitmentPath at a specified height.
// The caller is expected to construct the full CommitmentPath from a CommitmentPrefix and a standardized path (as defined in ICS 24).
func (cs *ClientState) VerifyNonMembership(
	ctx sdk.Context,
	clientStore sdk.KVStore,
	cdc codec.BinaryCodec,
	height exported.Height,
	delayTimePeriod uint64,
	delayBlockPeriod uint64,
	proof []byte,
	path []byte,
) error {
	// TODO: Implement 06-solomachine VerifyNonMembership
	return nil
}

// produceVerificationArgs perfoms the basic checks on the arguments that are
// shared between the verification functions and returns the public key of the
// consensus state, the unmarshalled proof representing the signature and timestamp
// along with the solo-machine sequence encoded in the proofHeight.
func produceVerificationArgs(
	cdc codec.BinaryCodec,
	cs *ClientState,
	height exported.Height,
	prefix exported.Prefix,
	proof []byte,
) (cryptotypes.PubKey, signing.SignatureData, uint64, uint64, error) {
	if revision := height.GetRevisionNumber(); revision != 0 {
		return nil, nil, 0, 0, sdkerrors.Wrapf(sdkerrors.ErrInvalidHeight, "revision must be 0 for solomachine, got revision-number: %d", revision)
	}
	// sequence is encoded in the revision height of height struct
	sequence := height.GetRevisionHeight()
	if prefix == nil {
		return nil, nil, 0, 0, sdkerrors.Wrap(commitmenttypes.ErrInvalidPrefix, "prefix cannot be empty")
	}

	_, ok := prefix.(*commitmenttypes.MerklePrefix)
	if !ok {
		return nil, nil, 0, 0, sdkerrors.Wrapf(commitmenttypes.ErrInvalidPrefix, "invalid prefix type %T, expected MerklePrefix", prefix)
	}

	if proof == nil {
		return nil, nil, 0, 0, sdkerrors.Wrap(ErrInvalidProof, "proof cannot be empty")
	}

	timestampedSigData := &TimestampedSignatureData{}
	if err := cdc.Unmarshal(proof, timestampedSigData); err != nil {
		return nil, nil, 0, 0, sdkerrors.Wrapf(err, "failed to unmarshal proof into type %T", timestampedSigData)
	}

	timestamp := timestampedSigData.Timestamp

	if len(timestampedSigData.SignatureData) == 0 {
		return nil, nil, 0, 0, sdkerrors.Wrap(ErrInvalidProof, "signature data cannot be empty")
	}

	sigData, err := UnmarshalSignatureData(cdc, timestampedSigData.SignatureData)
	if err != nil {
		return nil, nil, 0, 0, err
	}

	if cs.ConsensusState == nil {
		return nil, nil, 0, 0, sdkerrors.Wrap(clienttypes.ErrInvalidConsensus, "consensus state cannot be empty")
	}

	latestSequence := cs.GetLatestHeight().GetRevisionHeight()
	if latestSequence != sequence {
		return nil, nil, 0, 0, sdkerrors.Wrapf(
			sdkerrors.ErrInvalidHeight,
			"client state sequence != proof sequence (%d != %d)", latestSequence, sequence,
		)
	}

	if cs.ConsensusState.GetTimestamp() > timestamp {
		return nil, nil, 0, 0, sdkerrors.Wrapf(ErrInvalidProof, "the consensus state timestamp is greater than the signature timestamp (%d >= %d)", cs.ConsensusState.GetTimestamp(), timestamp)
	}

	publicKey, err := cs.ConsensusState.GetPubKey()
	if err != nil {
		return nil, nil, 0, 0, err
	}

	return publicKey, sigData, timestamp, sequence, nil
}

// sets the client state to the store
func setClientState(store sdk.KVStore, cdc codec.BinaryCodec, clientState exported.ClientState) {
	bz := clienttypes.MustMarshalClientState(cdc, clientState)
	store.Set([]byte(host.KeyClientState), bz)
}
