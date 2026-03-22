import logging

# Ensure __init__.py allows access or just import directly
from api.proto import CONTRACTS_pb2
from api.proto import CONTRACTS_pb2_grpc

class IntelligenceService(CONTRACTS_pb2_grpc.IntelligenceServiceServicer):
    """
    gRPC Servicer class for the Python AI Brain.
    Strictly follows CANONICAL.md: Stateless execution, snake_case methods in python but matches proto definitions.
    """
    async def AnalyzeToken(self, request: CONTRACTS_pb2.TokenRequest, context) -> CONTRACTS_pb2.VerdictResponse:
        mint_address = request.mint_address
        creator_address = request.creator_address
        
        logging.info(f"Received AnalyzeToken request => mint: {mint_address}, creator: {creator_address}")
        
        # MOCK LOGIC for PR-05 - Ensures End-to-End connection works
        # Strict Stateless Rule: Variables are kept in function scope without disk/storage interactions
        score = 85
        verdict = "SAFE"
        reason = "Initial AI heuristic mock passed"
        
        logging.info(f"Response Verdict -> Score: {score}, Verdict: {verdict}")
        
        return CONTRACTS_pb2.VerdictResponse(
            score=score,
            verdict=verdict,
            reason=reason
        )
