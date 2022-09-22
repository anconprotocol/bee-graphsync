package api

import (
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
	"github.com/gorilla/mux"
)

type getStakeResponse struct {
	StakedAmount *big.Int `json:"stakedAmount"`
}

func (s *Service) stakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	overlayAddr, err := hex.DecodeString(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debug("get stake: decode overlay string failed", "string", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: decode overlay string failed")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}
	if len(overlayAddr) == 0 {
		overlayAddr = s.overlay.Bytes()
	}

	stakedAmount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "deposit stake: invalid amount")
		jsonhttp.BadRequest(w, "invalid staking amount")
		return
	}
	err = s.stakingContract.DepositStake(r.Context(), stakedAmount, overlayAddr)
	if err != nil {
		if errors.Is(err, stakingcontract.ErrInvalidStakeAmount) {
			s.logger.Debug("deposit stake: invalid stake amount", "error", err)
			s.logger.Error(nil, "deposit stake: invalid stake amount")
			jsonhttp.BadRequest(w, "minimum 1 BZZ required for staking")
			return
		}
		if errors.Is(err, stakingcontract.ErrInsufficientFunds) {
			s.logger.Debug("deposit stake: out of funds", "error", err)
			s.logger.Error(nil, "deposit stake: out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		s.logger.Debug("deposit stake: deposit failed", "error", err)
		s.logger.Error(nil, "deposit stake: deposit failed")
		jsonhttp.InternalServerError(w, "cannot stake")
		return
	}
	jsonhttp.OK(w, nil)
}

func (s *Service) getStakedAmountHandler(w http.ResponseWriter, r *http.Request) {
	overlayAddr, err := hex.DecodeString(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debug("get stake: decode overlay string failed", "string", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: decode overlay string failed")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}
	if len(overlayAddr) == 0 {
		overlayAddr = s.overlay.Bytes()
	}

	stakedAmount, err := s.stakingContract.GetStake(r.Context(), overlayAddr)
	if err != nil {
		s.logger.Debug("get stake: get staked amount failed", "overlayAddr", hex.EncodeToString(overlayAddr), "error", err)
		s.logger.Error(nil, "get stake: get staked amount failed")
		jsonhttp.InternalServerError(w, " get staked amount failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{StakedAmount: stakedAmount})
}

func (s *Service) defaultStakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	overlayAddr := s.overlay.Bytes()
	stakedAmount := big.NewInt(1)

	err := s.stakingContract.DepositStake(r.Context(), stakedAmount, overlayAddr)
	if err != nil {
		if errors.Is(err, stakingcontract.ErrInvalidStakeAmount) {
			s.logger.Debug("deposit stake: invalid stake amount", "error", err)
			s.logger.Error(nil, "deposit stake: invalid stake amount")
			jsonhttp.BadRequest(w, "minimum 1 BZZ required for staking")
			return
		}
		if errors.Is(err, stakingcontract.ErrInsufficientFunds) {
			s.logger.Debug("deposit stake: out of funds", "error", err)
			s.logger.Error(nil, "deposit stake: out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		s.logger.Debug("deposit stake: deposit failed", "error", err)
		s.logger.Error(nil, "deposit stake: deposit failed")
		jsonhttp.InternalServerError(w, "cannot stake")
		return
	}
	jsonhttp.OK(w, nil)
}
