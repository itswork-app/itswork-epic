import asyncio
import logging
import os
import sys

# Ensure the root directory is accessible so packages like 'api' and 'internal' can be imported natively
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '../../')))

import grpc
from api.proto import CONTRACTS_pb2_grpc
from internal.brain.services import IntelligenceService

async def serve() -> None:
    # Structured Logging Format for Python matching standard logging outputs
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    # Initialize high performance asynchronous gRPC server
    server = grpc.aio.server()
    
    # Register the stateless intelligence service logic to this server
    CONTRACTS_pb2_grpc.add_IntelligenceServiceServicer_to_server(IntelligenceService(), server)
    
    port = os.getenv("PORT", "50051")
    listen_addr = f'[::]:{port}'
    server.add_insecure_port(listen_addr)
    
    logging.info(f"Starting async gRPC Python Brain Server on {listen_addr}")
    await server.start()
    
    try:
        # Blocks the thread while serving asynchronously
        await server.wait_for_termination()
    except asyncio.CancelledError:
        logging.info("Shutting down gRPC server gracefully...")
        # Graceful shutdown allowing pending requests up to 5 seconds to finish
        await server.stop(grace=5.0)

if __name__ == '__main__':
    try:
        asyncio.run(serve())
    except KeyboardInterrupt:
        logging.info("Server exiting appropriately.")
