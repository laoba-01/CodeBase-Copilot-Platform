import grpc
from concurrent import futures
import embedding_pb2
import embedding_pb2_grpc
from embedding_service import EmbeddingService


class EmbeddingServicer(embedding_pb2_grpc.EmbeddingServiceServicer):
    def __init__(self):
        self.svc = EmbeddingService()

    def Embed(self, request, context):
        vectors = self.svc.embed(list(request.texts))
        response = embedding_pb2.EmbedResponse()
        for v in vectors:
            emb = embedding_pb2.Embedding()
            emb.values.extend(v)
            response.vectors.append(emb)
        return response

    def Rerank(self, request, context):
        results = self.svc.rerank(
            request.query,
            list(request.documents),
            request.top_k or 10,
        )
        response = embedding_pb2.RerankResponse()
        for r in results:
            response.results.append(
                embedding_pb2.RerankResult(index=r["index"], score=r["score"])
            )
        return response


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
    embedding_pb2_grpc.add_EmbeddingServiceServicer_to_server(
        EmbeddingServicer(), server
    )
    server.add_insecure_port("[::]:50051")
    server.start()
    print("Embedding server listening on :50051")
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
