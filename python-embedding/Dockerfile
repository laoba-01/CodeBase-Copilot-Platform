FROM pytorch/pytorch:2.1.2-cuda12.1-cudnn8-runtime

WORKDIR /app
COPY python-embedding/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY proto/embedding.proto proto/
RUN python -m grpc_tools.protoc -I proto --python_out=. --grpc_python_out=. proto/embedding.proto

COPY python-embedding/*.py .

EXPOSE 50051
CMD ["python", "server.py"]
