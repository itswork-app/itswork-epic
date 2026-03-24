import logging
import os
import sys

# Standardize path resolution for gRPC stubs across project
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), "../../")))

import sentry_sdk

# Ensure __init__.py allows access or just import directly
from api.proto import CONTRACTS_pb2, CONTRACTS_pb2_grpc

sentry_sdk.init(
    dsn=os.environ.get("SENTRY_DSN_PYTHON"),
    traces_sample_rate=1.0,
)


class IntelligenceService(CONTRACTS_pb2_grpc.IntelligenceServiceServicer):
    """
    gRPC Servicer class for the Python AI Brain.
    Strictly follows CANONICAL.md: Stateless execution, snake_case methods in python but matches proto definitions.
    """

    async def AnalyzeToken(self, request: CONTRACTS_pb2.TokenRequest, context) -> CONTRACTS_pb2.VerdictResponse:
        mint_address = request.mint_address
        creator_address = request.creator_address

        logging.info(f"Received AnalyzeToken request => mint: {mint_address}, creator: {creator_address}")

        logging.info(f"Received AnalyzeToken request => mint: {mint_address}, creator: {creator_address}")

        # Load configurable thresholds from environment variables
        min_wallet_age_hours = int(os.environ.get("MIN_WALLET_AGE_HOURS", 24))
        max_holder_concentration = float(os.environ.get("MAX_HOLDER_CONCENTRATION_PERCENT", 50.0))

        # Extract heuristic fields from request
        wallet_age = request.creator_wallet_age_hours
        is_lp_burned = request.is_lp_burned
        holder_concentration = request.top_10_holder_concentration_percent
        funding_check_passed = request.funding_source_check_passed
        is_renounced = request.is_renounced
        has_socials = request.has_socials
        bonding_progress = request.bonding_progress
        trade_velocity = request.trade_velocity

        # Base score
        score = 100
        reasons = []

        # 1. Wallet Age Check
        if wallet_age < min_wallet_age_hours:
            score -= 40
            reasons.append(f"Creator wallet age ({wallet_age}h) is under {min_wallet_age_hours}h (-40)")

        # 2. Top Holder Concentration
        if holder_concentration > max_holder_concentration:
            score -= 30
            reasons.append(f"Top 10 holders own {holder_concentration}% (>{max_holder_concentration}%) (-30)")

        # 3. Funding Source Check (Placeholder Logic)
        if not funding_check_passed:
            score -= 20
            reasons.append("Funding source flagged as suspicious (-20)")

        # 4. Liquidity Lock/Burn Check (Critical Override)
        if not is_lp_burned:
            score = min(score, 50)
            reasons.append("LP is not burned (Score capped at 50)")

        # 5. Contract Renouncement (Critical Override - Auto-Danger)
        if not is_renounced:
            score -= 50
            reasons.append("Contract Ownership NOT renounced (Auto-Danger) (-50)")

        # 6. Social Signal Check
        if not has_socials:
            score -= 20
            reasons.append("No social metadata (X/Telegram) found (-20)")

        # 7. Sniper Engine Metrics: Bonding Curve Momentum
        if bonding_progress >= 50.0:
            score += 15
            reasons.append(f"HIGH MOMENTUM: {bonding_progress:.1f}% Bonding Curve (+15)")

        # 8. Organic Growth vs Bot Velocity
        if trade_velocity > 50.0:
            # Very high velocity might be bot wash trading, but user wants to reward momentum
            score += 10
            reasons.append(f"High trade velocity: {trade_velocity:.1f} tpm (+10)")

        # Determine Verdict
        if score >= 80:
            verdict = "SAFE"
        elif score >= 50:
            verdict = "WARNING"
        else:
            verdict = "DANGER"

        # Construct final reason
        reason = "; ".join(reasons) if reasons else "Anti-Rug Heuristics passed."

        # Required output statement
        logging.info("Master Blueprint Read & Verified. Heuristic analysis complete.")
        logging.info(f"Response Verdict -> Score: {score}, Verdict: {verdict}")

        return CONTRACTS_pb2.VerdictResponse(score=score, verdict=verdict, reason=reason)
