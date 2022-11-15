package ibctesting

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/stretchr/testify/require"

	clienttypes "github.com/cosmos/ibc-go/v6/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/v6/modules/core/23-commitment/types"
	host "github.com/cosmos/ibc-go/v6/modules/core/24-host"
	"github.com/cosmos/ibc-go/v6/modules/core/exported"
	solomachine "github.com/cosmos/ibc-go/v6/modules/light-clients/06-solomachine"
)

// Solomachine is a testing helper used to simulate a counterparty
// solo machine client.
type Solomachine struct {
	t *testing.T

	cdc         codec.BinaryCodec
	ClientID    string
	PrivateKeys []cryptotypes.PrivKey // keys used for signing
	PublicKeys  []cryptotypes.PubKey  // keys used for generating solo machine pub key
	PublicKey   cryptotypes.PubKey    // key used for verification
	Sequence    uint64
	Time        uint64
	Diversifier string
}

// NewSolomachine returns a new solomachine instance with an `nKeys` amount of
// generated private/public key pairs and a sequence starting at 1. If nKeys
// is greater than 1 then a multisig public key is used.
func NewSolomachine(t *testing.T, cdc codec.BinaryCodec, clientID, diversifier string, nKeys uint64) *Solomachine {
	privKeys, pubKeys, pk := GenerateKeys(t, nKeys)

	return &Solomachine{
		t:           t,
		cdc:         cdc,
		ClientID:    clientID,
		PrivateKeys: privKeys,
		PublicKeys:  pubKeys,
		PublicKey:   pk,
		Sequence:    1,
		Time:        10,
		Diversifier: diversifier,
	}
}

// GenerateKeys generates a new set of secp256k1 private keys and public keys.
// If the number of keys is greater than one then the public key returned represents
// a multisig public key. The private keys are used for signing, the public
// keys are used for generating the public key and the public key is used for
// solo machine verification. The usage of secp256k1 is entirely arbitrary.
// The key type can be swapped for any key type supported by the PublicKey
// interface, if needed. The same is true for the amino based Multisignature
// public key.
func GenerateKeys(t *testing.T, n uint64) ([]cryptotypes.PrivKey, []cryptotypes.PubKey, cryptotypes.PubKey) {
	require.NotEqual(t, uint64(0), n, "generation of zero keys is not allowed")

	privKeys := make([]cryptotypes.PrivKey, n)
	pubKeys := make([]cryptotypes.PubKey, n)
	for i := uint64(0); i < n; i++ {
		privKeys[i] = secp256k1.GenPrivKey()
		pubKeys[i] = privKeys[i].PubKey()
	}

	var pk cryptotypes.PubKey
	if len(privKeys) > 1 {
		// generate multi sig pk
		pk = kmultisig.NewLegacyAminoPubKey(int(n), pubKeys)
	} else {
		pk = privKeys[0].PubKey()
	}

	return privKeys, pubKeys, pk
}

// ClientState returns a new solo machine ClientState instance.
func (solo *Solomachine) ClientState() *solomachine.ClientState {
	return solomachine.NewClientState(solo.Sequence, solo.ConsensusState())
}

// ConsensusState returns a new solo machine ConsensusState instance
func (solo *Solomachine) ConsensusState() *solomachine.ConsensusState {
	publicKey, err := codectypes.NewAnyWithValue(solo.PublicKey)
	require.NoError(solo.t, err)

	return &solomachine.ConsensusState{
		PublicKey:   publicKey,
		Diversifier: solo.Diversifier,
		Timestamp:   solo.Time,
	}
}

// GetHeight returns an exported.Height with Sequence as RevisionHeight
func (solo *Solomachine) GetHeight() exported.Height {
	return clienttypes.NewHeight(0, solo.Sequence)
}

// CreateHeader generates a new private/public key pair and creates the
// necessary signature to construct a valid solo machine header.
// A new diversifier will be used as well
func (solo *Solomachine) CreateHeader(newDiversifier string) *solomachine.Header {
	// generate new private keys and signature for header
	newPrivKeys, newPubKeys, newPubKey := GenerateKeys(solo.t, uint64(len(solo.PrivateKeys)))

	publicKey, err := codectypes.NewAnyWithValue(newPubKey)
	require.NoError(solo.t, err)

	data := &solomachine.HeaderData{
		NewPubKey:      publicKey,
		NewDiversifier: newDiversifier,
	}

	dataBz, err := solo.cdc.Marshal(data)
	require.NoError(solo.t, err)

	signBytes := &solomachine.SignBytes{
		Sequence:    solo.Sequence,
		Timestamp:   solo.Time,
		Diversifier: solo.Diversifier,
		Path:        []byte(solomachinetypes.SentinelHeaderPath),
		Data:        dataBz,
	}

	bz, err := solo.cdc.Marshal(signBytes)
	require.NoError(solo.t, err)

	sig := solo.GenerateSignature(bz)

	header := &solomachine.Header{
		Sequence:       solo.Sequence,
		Timestamp:      solo.Time,
		Signature:      sig,
		NewPublicKey:   publicKey,
		NewDiversifier: newDiversifier,
	}

	// assumes successful header update
	solo.Sequence++
	solo.PrivateKeys = newPrivKeys
	solo.PublicKeys = newPubKeys
	solo.PublicKey = newPubKey
	solo.Diversifier = newDiversifier

	return header
}

// CreateMisbehaviour constructs testing misbehaviour for the solo machine client
// by signing over two different data bytes at the same sequence.
func (solo *Solomachine) CreateMisbehaviour() *solomachine.Misbehaviour {
	merklePath := solo.GetClientStatePath("counterparty")
	path, err := solo.cdc.Marshal(&merklePath)
	require.NoError(solo.t, err)

	data, err := solo.cdc.Marshal(solo.ClientState())
	require.NoError(solo.t, err)

	signBytes := &solomachine.SignBytes{
		Sequence:    solo.Sequence,
		Timestamp:   solo.Time,
		Diversifier: solo.Diversifier,
		Path:        path,
		Data:        data,
	}

	bz, err := solo.cdc.Marshal(signBytes)
	require.NoError(solo.t, err)

	sig := solo.GenerateSignature(bz)
	signatureOne := solomachine.SignatureAndData{
		Signature: sig,
		Path:      path,
		Data:      data,
		Timestamp: solo.Time,
	}

	// misbehaviour signaturess can have different timestamps
	solo.Time++

	merklePath = solo.GetConsensusStatePath("counterparty", clienttypes.NewHeight(0, 1))
	path, err = solo.cdc.Marshal(&merklePath)
	require.NoError(solo.t, err)

	data, err = solo.cdc.Marshal(solo.ConsensusState())
	require.NoError(solo.t, err)

	signBytes = &solomachine.SignBytes{
		Sequence:    solo.Sequence,
		Timestamp:   solo.Time,
		Diversifier: solo.Diversifier,
		Path:        path,
		Data:        data,
	}

	bz, err = solo.cdc.Marshal(signBytes)
	require.NoError(solo.t, err)

	sig = solo.GenerateSignature(bz)
	signatureTwo := solomachine.SignatureAndData{
		Signature: sig,
		Path:      path,
		Data:      data,
		Timestamp: solo.Time,
	}

	return &solomachine.Misbehaviour{
		ClientId:     solo.ClientID,
		Sequence:     solo.Sequence,
		SignatureOne: &signatureOne,
		SignatureTwo: &signatureTwo,
	}
}

// GenerateSignature uses the stored private keys to generate a signature
// over the sign bytes with each key. If the amount of keys is greater than
// 1 then a multisig data type is returned.
func (solo *Solomachine) GenerateSignature(signBytes []byte) []byte {
	sigs := make([]signing.SignatureData, len(solo.PrivateKeys))
	for i, key := range solo.PrivateKeys {
		sig, err := key.Sign(signBytes)
		require.NoError(solo.t, err)

		sigs[i] = &signing.SingleSignatureData{
			Signature: sig,
		}
	}

	var sigData signing.SignatureData
	if len(sigs) == 1 {
		// single public key
		sigData = sigs[0]
	} else {
		// generate multi signature data
		multiSigData := multisig.NewMultisig(len(sigs))
		for i, sig := range sigs {
			multisig.AddSignature(multiSigData, sig, i)
		}

		sigData = multiSigData
	}

	protoSigData := signing.SignatureDataToProto(sigData)
	bz, err := solo.cdc.Marshal(protoSigData)
	require.NoError(solo.t, err)

	return bz
}

// GetClientStatePath returns the commitment path for the client state.
func (solo *Solomachine) GetClientStatePath(counterpartyClientIdentifier string) commitmenttypes.MerklePath {
	path, err := commitmenttypes.ApplyPrefix(prefix, commitmenttypes.NewMerklePath(host.FullClientStatePath(counterpartyClientIdentifier)))
	require.NoError(solo.t, err)

	return path
}

// GetConsensusStatePath returns the commitment path for the consensus state.
func (solo *Solomachine) GetConsensusStatePath(counterpartyClientIdentifier string, consensusHeight exported.Height) commitmenttypes.MerklePath {
	path, err := commitmenttypes.ApplyPrefix(prefix, commitmenttypes.NewMerklePath(host.FullConsensusStatePath(counterpartyClientIdentifier, consensusHeight)))
	require.NoError(solo.t, err)

	return path
}

// GetConnectionStatePath returns the commitment path for the connection state.
func (solo *Solomachine) GetConnectionStatePath(connID string) commitmenttypes.MerklePath {
	connectionPath := commitmenttypes.NewMerklePath(host.ConnectionPath(connID))
	path, err := commitmenttypes.ApplyPrefix(prefix, connectionPath)
	require.NoError(solo.t, err)

	return path
}

// GetChannelStatePath returns the commitment path for that channel state.
func (solo *Solomachine) GetChannelStatePath(portID, channelID string) commitmenttypes.MerklePath {
	channelPath := commitmenttypes.NewMerklePath(host.ChannelPath(portID, channelID))
	path, err := commitmenttypes.ApplyPrefix(prefix, channelPath)
	require.NoError(solo.t, err)

	return path
}

// GetPacketCommitmentPath returns the commitment path for a packet commitment.
func (solo *Solomachine) GetPacketCommitmentPath(portID, channelID string) commitmenttypes.MerklePath {
	commitmentPath := commitmenttypes.NewMerklePath(host.PacketCommitmentPath(portID, channelID, solo.Sequence))
	path, err := commitmenttypes.ApplyPrefix(prefix, commitmentPath)
	require.NoError(solo.t, err)

	return path
}

// GetPacketAcknowledgementPath returns the commitment path for a packet acknowledgement.
func (solo *Solomachine) GetPacketAcknowledgementPath(portID, channelID string) commitmenttypes.MerklePath {
	ackPath := commitmenttypes.NewMerklePath(host.PacketAcknowledgementPath(portID, channelID, solo.Sequence))
	path, err := commitmenttypes.ApplyPrefix(prefix, ackPath)
	require.NoError(solo.t, err)

	return path
}

// GetPacketReceiptPath returns the commitment path for a packet receipt
// and an absent receipts.
func (solo *Solomachine) GetPacketReceiptPath(portID, channelID string) commitmenttypes.MerklePath {
	receiptPath := commitmenttypes.NewMerklePath(host.PacketReceiptPath(portID, channelID, solo.Sequence))
	path, err := commitmenttypes.ApplyPrefix(prefix, receiptPath)
	require.NoError(solo.t, err)

	return path
}

// GetNextSequenceRecvPath returns the commitment path for the next sequence recv counter.
func (solo *Solomachine) GetNextSequenceRecvPath(portID, channelID string) commitmenttypes.MerklePath {
	nextSequenceRecvPath := commitmenttypes.NewMerklePath(host.NextSequenceRecvPath(portID, channelID))
	path, err := commitmenttypes.ApplyPrefix(prefix, nextSequenceRecvPath)
	require.NoError(solo.t, err)

	return path
}
