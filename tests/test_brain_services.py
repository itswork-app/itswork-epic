import os
from unittest.mock import MagicMock

import pytest

from api.proto import CONTRACTS_pb2
from internal.brain.services import IntelligenceService


@pytest.fixture
def service():
    # Set default thresholds for testing
    os.environ["MIN_WALLET_AGE_HOURS"] = "24"
    os.environ["MAX_HOLDER_CONCENTRATION_PERCENT"] = "50.0"
    return IntelligenceService()


@pytest.fixture
def context():
    return MagicMock()


@pytest.mark.asyncio
async def test_analyze_token_safe_token(service, context):
    request = CONTRACTS_pb2.TokenRequest(
        mint_address="SAFE_MINT",
        creator_address="SAFE_CREATOR",
        creator_wallet_age_hours=48,  # > 24
        is_lp_burned=True,  # LP Burned
        top_10_holder_concentration_percent=30.0,  # < 50
        funding_source_check_passed=True,
    )

    response = await service.AnalyzeToken(request, context)

    assert response.score == 100
    assert response.verdict == "SAFE"
    assert "Anti-Rug Heuristics passed" in response.reason


@pytest.mark.asyncio
async def test_analyze_token_wallet_age_penalty(service, context):
    request = CONTRACTS_pb2.TokenRequest(
        mint_address="TEST_MINT",
        creator_address="TEST_CREATOR",
        creator_wallet_age_hours=12,  # < 24 penalty (-40)
        is_lp_burned=True,
        top_10_holder_concentration_percent=30.0,
        funding_source_check_passed=True,
    )

    response = await service.AnalyzeToken(request, context)

    assert response.score == 60  # 100 - 40
    assert response.verdict == "WARNING"
    assert "under 24h" in response.reason


@pytest.mark.asyncio
async def test_analyze_token_holder_concentration_penalty(service, context):
    request = CONTRACTS_pb2.TokenRequest(
        mint_address="TEST_MINT",
        creator_address="TEST_CREATOR",
        creator_wallet_age_hours=48,
        is_lp_burned=True,
        top_10_holder_concentration_percent=60.0,  # > 50 penalty (-30)
        funding_source_check_passed=True,
    )

    response = await service.AnalyzeToken(request, context)

    assert response.score == 70  # 100 - 30
    assert response.verdict == "WARNING"
    assert "Top 10 holders own" in response.reason


@pytest.mark.asyncio
async def test_analyze_token_lp_not_burned_cap(service, context):
    # Even a perfect token otherwise should be capped at 50 if LP is not burned
    request = CONTRACTS_pb2.TokenRequest(
        mint_address="TEST_MINT",
        creator_address="TEST_CREATOR",
        creator_wallet_age_hours=48,
        is_lp_burned=False,  # Penalty: Cap score at 50
        top_10_holder_concentration_percent=30.0,
        funding_source_check_passed=True,
    )

    response = await service.AnalyzeToken(request, context)

    assert response.score == 50
    assert response.verdict == "WARNING"
    assert "LP is not burned" in response.reason


@pytest.mark.asyncio
async def test_analyze_token_multiple_penalties(service, context):
    request = CONTRACTS_pb2.TokenRequest(
        mint_address="TEST_MINT",
        creator_address="TEST_CREATOR",
        creator_wallet_age_hours=10,  # Penalty (-40)
        is_lp_burned=True,
        top_10_holder_concentration_percent=80.0,  # Penalty (-30)
        funding_source_check_passed=False,  # Penalty (-20)
    )

    response = await service.AnalyzeToken(request, context)

    assert response.score == 10  # 100 - 40 - 30 - 20
    assert response.verdict == "DANGER"
    assert "under 24h" in response.reason
    assert "Top 10 holders own" in response.reason
    assert "Funding source flagged" in response.reason
