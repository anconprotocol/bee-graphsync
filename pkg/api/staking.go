// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

func (s *Service) stakingAccessHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.stakingSem.TryAcquire(1) {
			s.logger.Debug("staking access: simultaneous on-chain operations not supported")
			s.logger.Error(nil, "staking access: simultaneous on-chain operations not supported")
			jsonhttp.TooManyRequests(w, "simultaneous on-chain operations not supported")
			return
		}
		defer s.stakingSem.Release(1)

		h.ServeHTTP(w, r)
	})
}

type getStakeResponse struct {
	StakedAmount *big.Int `json:"stakedAmount"`
}

func (s *Service) stakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_stake_deposit").Build()

	paths := struct {
		Address swarm.Address `map:"reference" validate:"required"`
		Amount  *big.Int      `map:"amount" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	err := s.stakingContract.DepositStake(r.Context(), paths.Amount, paths.Address)
	if err != nil {
		if errors.Is(err, stakingcontract.ErrInsufficientStakeAmount) {
			logger.Debug("insufficient stake amount", "error", err)
			logger.Error(nil, "insufficient stake amount")
			jsonhttp.BadRequest(w, "minimum 1 BZZ required for staking")
			return
		}
		if errors.Is(err, stakingcontract.ErrNotImplemented) {
			logger.Debug("not implemented", "error", err)
			logger.Error(nil, "not implemented")
			jsonhttp.NotImplemented(w, "not implemented")
			return
		}
		if errors.Is(err, stakingcontract.ErrInsufficientFunds) {
			logger.Debug("out of funds", "error", err)
			logger.Error(nil, "out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		logger.Debug("deposit failed", "error", err)
		logger.Error(nil, "deposit failed")
		jsonhttp.InternalServerError(w, "cannot stake")
		return
	}
	jsonhttp.OK(w, nil)
}

func (s *Service) getStakedAmountHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_stake").Build()

	paths := struct {
		Address swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	stakedAmount, err := s.stakingContract.GetStake(r.Context(), paths.Address)
	if err != nil {
		logger.Debug("get staked amount failed", "overlayAddr", paths.Address, "error", err)
		logger.Error(nil, "get staked amount failed")
		jsonhttp.InternalServerError(w, "get staked amount failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{StakedAmount: stakedAmount})
}
