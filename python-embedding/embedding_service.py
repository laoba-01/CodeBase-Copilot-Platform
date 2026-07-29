import os
from typing import List


class EmbeddingService:
    def __init__(self, model_name: str = "BAAI/bge-m3"):
        # Use local path if MODEL_PATH env var is set
        local_path = os.environ.get("MODEL_PATH", "")
        if local_path and os.path.isdir(local_path):
            self._model_name = local_path
            print(f"Using local model: {local_path}")
        else:
            self._model_name = model_name
            print(f"Will load model: {model_name}")
        self.model = None
        self._backend = None

    def _ensure_model(self):
        if self.model is not None:
            return

        from sentence_transformers import SentenceTransformer

        # Try ModelScope first (works in China without proxy)
        try:
            from modelscope.hub.snapshot_download import snapshot_download
            print(f"Downloading {self._model_name} via ModelScope...")
            local_dir = snapshot_download(
                self._model_name,
                cache_dir=os.environ.get("MODELSCOPE_CACHE", "/tmp/modelscope"),
            )
            print(f"Model downloaded to: {local_dir}")
            self.model = SentenceTransformer(local_dir, device="cpu")
            self._backend = "modelscope+sentence_transformers"
        except Exception as e:
            print(f"ModelScope failed: {e}, trying direct SentenceTransformer...")
            self.model = SentenceTransformer(self._model_name, device="cpu")
            self._backend = "sentence_transformers"

        print(f"Model loaded. Backend: {self._backend}")

    def embed(self, texts: List[str]) -> List[List[float]]:
        self._ensure_model()
        embeddings = self.model.encode(
            texts, batch_size=32, show_progress_bar=False,
            normalize_embeddings=False,
        )
        return embeddings.tolist()

    def rerank(self, query: str, documents: List[str], top_k: int = 10) -> List[dict]:
        self._ensure_model()
        from sentence_transformers import CrossEncoder
        reranker = CrossEncoder(
            self._model_name,
            device="cpu",
        )
        pairs = [[query, doc] for doc in documents]
        scores = reranker.predict(pairs, show_progress_bar=False)
        ranked = sorted(
            [{"index": i, "score": float(s)} for i, s in enumerate(scores)],
            key=lambda x: x["score"], reverse=True,
        )
        return ranked[:top_k]
