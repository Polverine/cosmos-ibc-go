package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/ibc-go/modules/apps/29-fee/types"
)

func (suite *KeeperTestSuite) TestRegisterCounterpartyAddress() {
	var (
		addr  string
		addr2 string
	)

	testCases := []struct {
		name     string
		expPass  bool
		malleate func()
	}{
		{
			"success",
			true,
			func() {},
		},
	}

	for _, tc := range testCases {
		suite.SetupTest()
		ctx := suite.chainA.GetContext()

		addr = suite.chainA.SenderAccount.GetAddress().String()
		addr2 = suite.chainB.SenderAccount.GetAddress().String()
		msg := types.NewMsgRegisterCounterpartyAddress(addr, addr2)
		tc.malleate()

		_, err := suite.chainA.SendMsgs(msg)

		if tc.expPass {
			suite.Require().NoError(err) // message committed

			counterpartyAddress, _ := suite.chainA.GetSimApp().IBCFeeKeeper.GetCounterpartyAddress(ctx, suite.chainA.SenderAccount.GetAddress())
			suite.Require().Equal(addr2, counterpartyAddress.String())
		} else {
			suite.Require().Error(err)
		}
	}
}

func (suite *KeeperTestSuite) TestPayPacketFee() {
	testCases := []struct {
		name     string
		expPass  bool
		malleate func()
	}{
		{
			"success",
			true,
			func() {},
		},
	}

	for _, tc := range testCases {
		suite.SetupTest()
		suite.coordinator.SetupConnections(suite.path)
		SetupFeePath(suite.path)
		refundAcc := suite.chainA.SenderAccount.GetAddress()
		validCoin := &sdk.Coin{Denom: sdk.DefaultBondDenom, Amount: sdk.NewInt(100)}
		ackFee := validCoin
		receiveFee := validCoin
		timeoutFee := validCoin

		fee := &types.Fee{ackFee, receiveFee, timeoutFee}
		msg := types.NewMsgPayPacketFee(fee, suite.path.EndpointA.ChannelConfig.PortID, suite.path.EndpointA.ChannelID, refundAcc.String(), []string{})

		tc.malleate()
		_, err := suite.chainA.SendMsgs(msg)
		suite.Require().NoError(err) // message committed

		if tc.expPass {
			suite.Require().NoError(err) // message committed
		} else {
			suite.Require().Error(err)
		}
	}
}
