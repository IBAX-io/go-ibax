/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package bft is the consensus of a chain whose first block sets bft_consensus = 1 (F18): the
// validators commit each block by the precommits of more than two thirds of them, after the
// algorithm of Buchman, Kwon and Milosevic, "The latest gossip on BFT consensus",
// arXiv:1807.04938, and a committed block is final.
//
// This file set holds what the consensus signs and what the chain checks: the quorum of a set of
// validators, the bytes a validator signs for a proposal or a vote, the wire form of both, and
// the evidence of a validator that signed two messages of the same height, round and type. The
// encodings are those of section 4.5 of the design (ibax-project
// docs/plans/2026-10-20-f18-federal-bft-consensus-design.md): every integer big-endian of fixed
// length, every byte string after a uint16 length.
package bft
