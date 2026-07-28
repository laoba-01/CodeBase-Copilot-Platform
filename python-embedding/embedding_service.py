from typing import List


class EmbeddingService:
    def __init__(self, model_name: str = "BAAI/bge-m3"):
        print(f"Loading model: {model_name}")
        self.model = None  # Lazy load to avoid import overhead at module level
        self._model_name = model_name

    def _ensure_model(self):
        if self.model is None:
            from FlagEmbedding import BGEM3FlagModel

            print(f"Loading model: {self._model_name}")
            self.model = BGEM3FlagModel(self._model_name, use_fp16=True)
            print("Model loaded.")

    def embed(self, texts: List[str]) -> List[List[float]]:
        """Encode texts to 1024-dim vectors (dense outputs)."""
        self._ensure_model()
        outputs = self.model.encode(
            texts,
            batch_size=32,
            max_length=512,
            return_dense=True,
            return_sparse=False,
        )
        # BGEM3 outputs dense_vecs as numpy arrays
        return outputs["dense_vecs"].tolist()

    def rerank(self, query: str, documents: List[str], top_k: int = 10) -> List[dict]:
        """Rerank documents by relevance to query using built-in cross-encoder."""
        self._ensure_model()
        # Use model's built-in cross-encoder scoring
        pairs = [[query, doc] for doc in documents]
        scores = self.model.compute_score(
            pairs,
            batch_size=32,
            max_length=512,
        )
        # scores is a list of floats
        ranked = sorted(
            [{"index": i, "score": float(s)} for i, s in enumerate(scores)],
            key=lambda x: x["score"],
            reverse=True,
        )
        return ranked[:top_k]
