package keeper_test

import (
	"encoding/hex"
	"fmt"
	"time"

	tmtypes "github.com/tendermint/tendermint/types"

	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/cosmos/ibc-go/modules/core/02-client/types"
	clienttypes "github.com/cosmos/ibc-go/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/modules/core/23-commitment/types"
	"github.com/cosmos/ibc-go/modules/core/exported"
	ibctmtypes "github.com/cosmos/ibc-go/modules/light-clients/07-tendermint/types"
	localhosttypes "github.com/cosmos/ibc-go/modules/light-clients/09-localhost/types"
	ibctesting "github.com/cosmos/ibc-go/testing"
	ibctestingmock "github.com/cosmos/ibc-go/testing/mock"
)

func (suite *KeeperTestSuite) TestCreateClient() {
	cases := []struct {
		msg         string
		clientState exported.ClientState
		expPass     bool
	}{
		{"success", ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false), true},
		{"client type not supported", localhosttypes.NewClientState(testChainID, clienttypes.NewHeight(0, 1)), false},
	}

	for i, tc := range cases {

		clientID, err := suite.keeper.CreateClient(suite.ctx, tc.clientState, suite.consensusState)
		if tc.expPass {
			suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.msg)
			suite.Require().NotNil(clientID, "valid test case %d failed: %s", i, tc.msg)
		} else {
			suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.msg)
			suite.Require().Equal("", clientID, "invalid test case %d passed: %s", i, tc.msg)
		}
	}
}

func (suite *KeeperTestSuite) TestUpdateClientTendermint() {
	// Must create header creation functions since suite.header gets recreated on each test case
	createFutureUpdateFn := func(s *KeeperTestSuite) *ibctmtypes.Header {
		heightPlus3 := clienttypes.NewHeight(suite.header.GetHeight().GetRevisionNumber(), suite.header.GetHeight().GetRevisionHeight()+3)
		height := suite.header.GetHeight().(clienttypes.Height)

		return suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus3.RevisionHeight), height, suite.header.Header.Time.Add(time.Hour),
			suite.valSet, suite.valSet, []tmtypes.PrivValidator{suite.privVal})
	}
	createPastUpdateFn := func(s *KeeperTestSuite) *ibctmtypes.Header {
		heightMinus2 := clienttypes.NewHeight(suite.header.GetHeight().GetRevisionNumber(), suite.header.GetHeight().GetRevisionHeight()-2)
		heightMinus4 := clienttypes.NewHeight(suite.header.GetHeight().GetRevisionNumber(), suite.header.GetHeight().GetRevisionHeight()-4)

		return suite.chainA.CreateTMClientHeader(testChainID, int64(heightMinus2.RevisionHeight), heightMinus4, suite.header.Header.Time,
			suite.valSet, suite.valSet, []tmtypes.PrivValidator{suite.privVal})
	}
	var (
		updateHeader *ibctmtypes.Header
		clientState  *ibctmtypes.ClientState
		clientID     string
		err          error
	)

	cases := []struct {
		name     string
		malleate func() error
		expPass  bool
		freeze   bool // only true if update freezes the client to a new frozen height. false if client is already frozen and header is valid past update
	}{
		{"valid update", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

			// store intermediate consensus state to check that trustedHeight does not need to be highest consensus state before header height
			incrementedClientHeight := testClientHeight.Increment().(types.Height)
			intermediateConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.now.Add(time.Minute),
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, incrementedClientHeight, intermediateConsState)

			clientState.LatestHeight = incrementedClientHeight
			suite.keeper.SetClientState(suite.ctx, clientID, clientState)

			updateHeader = createFutureUpdateFn(suite)
			return err
		}, true, false},
		{"valid past update", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)
			suite.Require().NoError(err)

			height1 := types.NewHeight(0, 1)

			// store previous consensus state
			prevConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.past,
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, height1, prevConsState)

			height2 := types.NewHeight(0, 2)

			// store intermediate consensus state to check that trustedHeight does not need to be hightest consensus state before header height
			intermediateConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.past.Add(time.Minute),
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, height2, intermediateConsState)

			// updateHeader will fill in consensus state between prevConsState and suite.consState
			// clientState should not be updated
			updateHeader = createPastUpdateFn(suite)
			return nil
		}, true, false},
		{"valid duplicate update", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)
			suite.Require().NoError(err)

			height1 := types.NewHeight(0, 1)

			// store previous consensus state
			prevConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.past,
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, height1, prevConsState)

			height2 := types.NewHeight(0, 2)

			// store intermediate consensus state to check that trustedHeight does not need to be hightest consensus state before header height
			intermediateConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.past.Add(time.Minute),
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, height2, intermediateConsState)

			// updateHeader will fill in consensus state between prevConsState and suite.consState
			// clientState should not be updated
			updateHeader = createPastUpdateFn(suite)
			// set updateHeader's consensus state in store to create duplicate UpdateClient scenario
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, updateHeader.GetHeight(), updateHeader.ConsensusState())
			return nil
		}, true, false},
		{"misbehaviour detection: conflicting header", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)
			suite.Require().NoError(err)

			height1 := types.NewHeight(0, 1)

			// store previous consensus state
			prevConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.past,
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, height1, prevConsState)

			height2 := types.NewHeight(0, 2)

			// store intermediate consensus state to check that trustedHeight does not need to be hightest consensus state before header height
			intermediateConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.past.Add(time.Minute),
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, height2, intermediateConsState)

			// updateHeader will fill in consensus state between prevConsState and suite.consState
			// clientState should not be updated
			updateHeader = createPastUpdateFn(suite)
			// set conflicting consensus state in store to create misbehaviour scenario
			conflictConsState := updateHeader.ConsensusState()
			conflictConsState.Root = commitmenttypes.NewMerkleRoot([]byte("conflicting apphash"))
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, updateHeader.GetHeight(), conflictConsState)
			return nil
		}, true, true},
		{"misbehaviour detection: monotonic time violation", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

			// store intermediate consensus state at a time greater than updateHeader time
			// this will break time monotonicity
			incrementedClientHeight := testClientHeight.Increment().(types.Height)
			intermediateConsState := &ibctmtypes.ConsensusState{
				Timestamp:          suite.header.Header.Time.Add(2 * time.Hour),
				NextValidatorsHash: suite.valSetHash,
			}
			suite.keeper.SetClientConsensusState(suite.ctx, clientID, incrementedClientHeight, intermediateConsState)
			// set iteration key
			clientStore := suite.keeper.ClientStore(suite.ctx, clientID)
			ibctmtypes.SetIterationKey(clientStore, incrementedClientHeight)

			clientState.LatestHeight = incrementedClientHeight
			suite.keeper.SetClientState(suite.ctx, clientID, clientState)

			updateHeader = createFutureUpdateFn(suite)
			return err
		}, true, true},
		{"client state not found", func() error {
			updateHeader = createFutureUpdateFn(suite)

			return nil
		}, false, false},
		{"consensus state not found", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			suite.keeper.SetClientState(suite.ctx, testClientID, clientState)
			updateHeader = createFutureUpdateFn(suite)

			return nil
		}, false, false},
		{"frozen client before update", func() error {
			clientState = &ibctmtypes.ClientState{FrozenHeight: types.NewHeight(0, 1), LatestHeight: testClientHeight}
			suite.keeper.SetClientState(suite.ctx, testClientID, clientState)
			updateHeader = createFutureUpdateFn(suite)

			return nil
		}, false, false},
		{"invalid header", func() error {
			clientState = ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
			_, err := suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)
			suite.Require().NoError(err)
			updateHeader = createPastUpdateFn(suite)

			return nil
		}, false, false},
	}

	for i, tc := range cases {
		tc := tc
		i := i
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest()
			clientID = testClientID // must be explicitly changed

			err := tc.malleate()
			suite.Require().NoError(err)

			suite.ctx = suite.ctx.WithBlockTime(updateHeader.Header.Time.Add(time.Minute))

			err = suite.keeper.UpdateClient(suite.ctx, clientID, updateHeader)

			if tc.expPass {
				suite.Require().NoError(err, err)

				newClientState, found := suite.keeper.GetClientState(suite.ctx, clientID)
				suite.Require().True(found, "valid test case %d failed: %s", i, tc.name)

				// If the update freezes the client, check that client was frozen to update header's height.
				// Otherwise check that consensus state is stored as expected.
				if tc.freeze {
					suite.Require().True(newClientState.IsFrozen(), "client did not freeze after conflicting header was submitted to UpdateClient")
					suite.Require().Equal(newClientState.GetFrozenHeight(), updateHeader.GetHeight(), "client frozen at wrong height")
				} else {
					expConsensusState := &ibctmtypes.ConsensusState{
						Timestamp:          updateHeader.GetTime(),
						Root:               commitmenttypes.NewMerkleRoot(updateHeader.Header.GetAppHash()),
						NextValidatorsHash: updateHeader.Header.NextValidatorsHash,
					}

					consensusState, found := suite.keeper.GetClientConsensusState(suite.ctx, clientID, updateHeader.GetHeight())
					suite.Require().True(found, "valid test case %d failed: %s", i, tc.name)

					// Determine if clientState should be updated or not
					if updateHeader.GetHeight().GT(clientState.GetLatestHeight()) {
						// Header Height is greater than clientState latest Height, clientState should be updated with header.GetHeight()
						suite.Require().Equal(updateHeader.GetHeight(), newClientState.GetLatestHeight(), "clientstate height did not update")
					} else {
						// Update will add past consensus state, clientState should not be updated at all
						suite.Require().Equal(clientState.GetLatestHeight(), newClientState.GetLatestHeight(), "client state height updated for past header")
					}

					suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.name)
					suite.Require().Equal(expConsensusState, consensusState, "consensus state should have been updated on case %s", tc.name)
				}
			} else {
				suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.name)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestUpdateClientLocalhost() {
	revision := types.ParseChainID(suite.chainA.ChainID)
	var localhostClient exported.ClientState = localhosttypes.NewClientState(suite.chainA.ChainID, types.NewHeight(revision, uint64(suite.chainA.GetContext().BlockHeight())))

	ctx := suite.chainA.GetContext().WithBlockHeight(suite.chainA.GetContext().BlockHeight() + 1)
	err := suite.chainA.App.GetIBCKeeper().ClientKeeper.UpdateClient(ctx, exported.Localhost, nil)
	suite.Require().NoError(err)

	clientState, found := suite.chainA.App.GetIBCKeeper().ClientKeeper.GetClientState(ctx, exported.Localhost)
	suite.Require().True(found)
	suite.Require().Equal(localhostClient.GetLatestHeight().(types.Height).Increment(), clientState.GetLatestHeight())
}

func (suite *KeeperTestSuite) TestUpgradeClient() {
	var (
		path                                        *ibctesting.Path
		upgradedClient                              exported.ClientState
		upgradedConsState                           exported.ConsensusState
		lastHeight                                  exported.Height
		proofUpgradedClient, proofUpgradedConsState []byte
		upgradedClientBz, upgradedConsStateBz       []byte
		err                                         error
	)

	testCases := []struct {
		name    string
		setup   func()
		expPass bool
	}{
		{
			name: "successful upgrade",
			setup: func() {
				// last Height is at next block
				lastHeight = clienttypes.NewHeight(0, uint64(suite.chainB.GetContext().BlockHeight()+1))

				// zero custom fields and store in upgrade store
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedClient(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedClientBz)
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedConsensusState(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedConsStateBz)

				// commit upgrade store changes and update clients

				suite.coordinator.CommitBlock(suite.chainB)
				err := path.EndpointA.UpdateClient()
				suite.Require().NoError(err)

				cs, found := suite.chainA.App.GetIBCKeeper().ClientKeeper.GetClientState(suite.chainA.GetContext(), path.EndpointA.ClientID)
				suite.Require().True(found)

				proofUpgradedClient, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedClientKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())
				proofUpgradedConsState, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedConsStateKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())
			},
			expPass: true,
		},
		{
			name: "client state not found",
			setup: func() {
				// last Height is at next block
				lastHeight = clienttypes.NewHeight(0, uint64(suite.chainB.GetContext().BlockHeight()+1))

				// zero custom fields and store in upgrade store
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedClient(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedClientBz)
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedConsensusState(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedConsStateBz)

				// commit upgrade store changes and update clients

				suite.coordinator.CommitBlock(suite.chainB)
				err := path.EndpointA.UpdateClient()
				suite.Require().NoError(err)

				cs, found := suite.chainA.App.GetIBCKeeper().ClientKeeper.GetClientState(suite.chainA.GetContext(), path.EndpointA.ClientID)
				suite.Require().True(found)

				proofUpgradedClient, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedClientKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())
				proofUpgradedConsState, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedConsStateKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())

				path.EndpointA.ClientID = "wrongclientid"
			},
			expPass: false,
		},
		{
			name: "client state frozen",
			setup: func() {
				// last Height is at next block
				lastHeight = clienttypes.NewHeight(0, uint64(suite.chainB.GetContext().BlockHeight()+1))

				// zero custom fields and store in upgrade store
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedClient(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedClientBz)
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedConsensusState(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedConsStateBz)

				// commit upgrade store changes and update clients

				suite.coordinator.CommitBlock(suite.chainB)
				err := path.EndpointA.UpdateClient()
				suite.Require().NoError(err)

				cs, found := suite.chainA.App.GetIBCKeeper().ClientKeeper.GetClientState(suite.chainA.GetContext(), path.EndpointA.ClientID)
				suite.Require().True(found)

				proofUpgradedClient, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedClientKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())
				proofUpgradedConsState, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedConsStateKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())

				// set frozen client in store
				tmClient, ok := cs.(*ibctmtypes.ClientState)
				suite.Require().True(ok)
				tmClient.FrozenHeight = types.NewHeight(0, 1)
				suite.chainA.App.GetIBCKeeper().ClientKeeper.SetClientState(suite.chainA.GetContext(), path.EndpointA.ClientID, tmClient)
			},
			expPass: false,
		},
		{
			name: "tendermint client VerifyUpgrade fails",
			setup: func() {
				// last Height is at next block
				lastHeight = clienttypes.NewHeight(0, uint64(suite.chainB.GetContext().BlockHeight()+1))

				// zero custom fields and store in upgrade store
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedClient(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedClientBz)
				suite.chainB.GetSimApp().UpgradeKeeper.SetUpgradedConsensusState(suite.chainB.GetContext(), int64(lastHeight.GetRevisionHeight()), upgradedConsStateBz)

				// change upgradedClient client-specified parameters
				upgradedClient = ibctmtypes.NewClientState("wrongchainID", ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, newClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, true, true)

				suite.coordinator.CommitBlock(suite.chainB)
				err := path.EndpointA.UpdateClient()
				suite.Require().NoError(err)

				cs, found := suite.chainA.App.GetIBCKeeper().ClientKeeper.GetClientState(suite.chainA.GetContext(), path.EndpointA.ClientID)
				suite.Require().True(found)

				proofUpgradedClient, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedClientKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())
				proofUpgradedConsState, _ = suite.chainB.QueryUpgradeProof(upgradetypes.UpgradedConsStateKey(int64(lastHeight.GetRevisionHeight())), cs.GetLatestHeight().GetRevisionHeight())
			},
			expPass: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		path = ibctesting.NewPath(suite.chainA, suite.chainB)
		suite.coordinator.SetupClients(path)
		upgradedClient = ibctmtypes.NewClientState("newChainId", ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod+trustingPeriod, maxClockDrift, newClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
		upgradedClient = upgradedClient.ZeroCustomFields()
		upgradedClientBz, err = types.MarshalClientState(suite.chainA.App.AppCodec(), upgradedClient)
		suite.Require().NoError(err)

		upgradedConsState = &ibctmtypes.ConsensusState{
			NextValidatorsHash: []byte("nextValsHash"),
		}
		upgradedConsStateBz, err = types.MarshalConsensusState(suite.chainA.App.AppCodec(), upgradedConsState)
		suite.Require().NoError(err)

		tc.setup()

		// Call ZeroCustomFields on upgraded clients to clear any client-chosen parameters in test-case upgradedClient
		upgradedClient = upgradedClient.ZeroCustomFields()

		err = suite.chainA.App.GetIBCKeeper().ClientKeeper.UpgradeClient(suite.chainA.GetContext(), path.EndpointA.ClientID, upgradedClient, upgradedConsState, proofUpgradedClient, proofUpgradedConsState)

		if tc.expPass {
			suite.Require().NoError(err, "verify upgrade failed on valid case: %s", tc.name)
		} else {
			suite.Require().Error(err, "verify upgrade passed on invalid case: %s", tc.name)
		}
	}

}

func (suite *KeeperTestSuite) TestCheckMisbehaviourAndUpdateState() {
	var (
		clientID string
		err      error
	)

	altPrivVal := ibctestingmock.NewPV()
	altPubKey, err := altPrivVal.GetPubKey()
	suite.Require().NoError(err)
	altVal := tmtypes.NewValidator(altPubKey, 4)

	// Set valSet here with suite.valSet so it doesn't get reset on each testcase
	valSet := suite.valSet
	valsHash := valSet.Hash()

	// Create bothValSet with both suite validator and altVal
	bothValSet := tmtypes.NewValidatorSet(append(suite.valSet.Validators, altVal))
	bothValsHash := bothValSet.Hash()
	// Create alternative validator set with only altVal
	altValSet := tmtypes.NewValidatorSet([]*tmtypes.Validator{altVal})

	// Create signer array and ensure it is in same order as bothValSet
	_, suiteVal := suite.valSet.GetByIndex(0)
	bothSigners := ibctesting.CreateSortedSignerArray(altPrivVal, suite.privVal, altVal, suiteVal)

	altSigners := []tmtypes.PrivValidator{altPrivVal}

	// Create valid Misbehaviour by making a duplicate header that signs over different block time
	altTime := suite.ctx.BlockTime().Add(time.Minute)

	heightPlus3 := types.NewHeight(0, height+3)
	heightPlus5 := types.NewHeight(0, height+5)

	testCases := []struct {
		name         string
		misbehaviour *ibctmtypes.Misbehaviour
		malleate     func() error
		expPass      bool
	}{
		{
			"trusting period misbehavior should pass",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, altTime, bothValSet, bothValSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, suite.ctx.BlockTime(), bothValSet, bothValSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = bothValsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				return err
			},
			true,
		},
		{
			"time misbehavior should pass",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+5), testClientHeight, suite.ctx.BlockTime(), bothValSet, bothValSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, altTime, bothValSet, bothValSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = bothValsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				return err
			},
			true,
		},
		{
			"misbehavior at later height should pass",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), testClientHeight, altTime, bothValSet, valSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), testClientHeight, suite.ctx.BlockTime(), bothValSet, valSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = valsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				// store intermediate consensus state to check that trustedHeight does not need to be highest consensus state before header height
				intermediateConsState := &ibctmtypes.ConsensusState{
					Timestamp:          suite.now.Add(time.Minute),
					NextValidatorsHash: suite.valSetHash,
				}
				suite.keeper.SetClientConsensusState(suite.ctx, clientID, heightPlus3, intermediateConsState)

				clientState.LatestHeight = heightPlus3
				suite.keeper.SetClientState(suite.ctx, clientID, clientState)

				return err
			},
			true,
		},
		{
			"misbehavior at later height with different trusted heights should pass",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), testClientHeight, altTime, bothValSet, valSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), heightPlus3, suite.ctx.BlockTime(), bothValSet, bothValSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = valsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				// store trusted consensus state for Header2
				intermediateConsState := &ibctmtypes.ConsensusState{
					Timestamp:          suite.now.Add(time.Minute),
					NextValidatorsHash: bothValsHash,
				}
				suite.keeper.SetClientConsensusState(suite.ctx, clientID, heightPlus3, intermediateConsState)

				clientState.LatestHeight = heightPlus3
				suite.keeper.SetClientState(suite.ctx, clientID, clientState)

				return err
			},
			true,
		},
		{
			"misbehavior ValidateBasic fails: misbehaviour height is at same height as trusted height",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight), testClientHeight, altTime, bothValSet, bothValSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight), testClientHeight, suite.ctx.BlockTime(), bothValSet, bothValSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = bothValsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				return err
			},
			false,
		},
		{
			"trusted ConsensusState1 not found",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), heightPlus3, altTime, bothValSet, bothValSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), testClientHeight, suite.ctx.BlockTime(), bothValSet, valSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = valsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)
				// intermediate consensus state at height + 3 is not created
				return err
			},
			false,
		},
		{
			"trusted ConsensusState2 not found",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), testClientHeight, altTime, bothValSet, valSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(heightPlus5.RevisionHeight+1), heightPlus3, suite.ctx.BlockTime(), bothValSet, bothValSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = valsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)
				// intermediate consensus state at height + 3 is not created
				return err
			},
			false,
		},
		{
			"client state not found",
			&ibctmtypes.Misbehaviour{},
			func() error { return nil },
			false,
		},
		{
			"client already frozen at earlier height",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, altTime, bothValSet, bothValSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, suite.ctx.BlockTime(), bothValSet, bothValSet, bothSigners),
				ClientId: clientID,
			},
			func() error {
				suite.consensusState.NextValidatorsHash = bothValsHash
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				clientState.FrozenHeight = types.NewHeight(0, 1)
				suite.keeper.SetClientState(suite.ctx, clientID, clientState)

				return err
			},
			false,
		},
		{
			"misbehaviour check failed",
			&ibctmtypes.Misbehaviour{
				Header1:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, altTime, bothValSet, bothValSet, bothSigners),
				Header2:  suite.chainA.CreateTMClientHeader(testChainID, int64(testClientHeight.RevisionHeight+1), testClientHeight, suite.ctx.BlockTime(), altValSet, bothValSet, altSigners),
				ClientId: clientID,
			},
			func() error {
				clientState := ibctmtypes.NewClientState(testChainID, ibctmtypes.DefaultTrustLevel, trustingPeriod, ubdPeriod, maxClockDrift, testClientHeight, commitmenttypes.GetSDKSpecs(), ibctesting.UpgradePath, false, false)
				if err != nil {
					return err
				}
				clientID, err = suite.keeper.CreateClient(suite.ctx, clientState, suite.consensusState)

				return err
			},
			false,
		},
	}

	for i, tc := range testCases {
		tc := tc
		i := i

		suite.Run(tc.name, func() {
			suite.SetupTest()       // reset
			clientID = testClientID // must be explicitly changed

			err := tc.malleate()
			suite.Require().NoError(err)

			tc.misbehaviour.ClientId = clientID

			err = suite.keeper.CheckMisbehaviourAndUpdateState(suite.ctx, tc.misbehaviour)

			if tc.expPass {
				suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.name)

				clientState, found := suite.keeper.GetClientState(suite.ctx, clientID)
				suite.Require().True(found, "valid test case %d failed: %s", i, tc.name)
				suite.Require().True(clientState.IsFrozen(), "valid test case %d failed: %s", i, tc.name)
				suite.Require().Equal(tc.misbehaviour.GetHeight(), clientState.GetFrozenHeight(),
					"valid test case %d failed: %s. Expected FrozenHeight %s got %s", tc.misbehaviour.GetHeight(), clientState.GetFrozenHeight())
			} else {
				suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.name)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestUpdateClientEventEmission() {
	path := ibctesting.NewPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupClients(path)
	header, err := suite.chainA.ConstructUpdateTMClientHeader(suite.chainB, path.EndpointA.ClientID)
	suite.Require().NoError(err)

	msg, err := clienttypes.NewMsgUpdateClient(
		path.EndpointA.ClientID, header,
		suite.chainA.SenderAccount.GetAddress().String(),
	)

	result, err := suite.chainA.SendMsgs(msg)
	suite.Require().NoError(err)
	// first event type is "message"
	updateEvent := result.Events[1]

	suite.Require().Equal(clienttypes.EventTypeUpdateClient, updateEvent.Type)

	// use a boolean to ensure the update event contains the header
	contains := false
	for _, attr := range updateEvent.Attributes {
		if string(attr.Key) == clienttypes.AttributeKeyHeader {
			contains = true

			bz, err := hex.DecodeString(string(attr.Value))
			suite.Require().NoError(err)

			emittedHeader, err := types.UnmarshalHeader(suite.chainA.App.AppCodec(), bz)
			suite.Require().NoError(err)
			suite.Require().Equal(header, emittedHeader)
		}

	}
	suite.Require().True(contains)

}
