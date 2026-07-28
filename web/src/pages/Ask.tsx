import React, { useState, useRef, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Input,
  Button,
  Card,
  Typography,
  Space,
  Tag,
  Spin,
  message,
  Alert,
} from 'antd';
import {
  SendOutlined,
  StopOutlined,
  ArrowLeftOutlined,
  RobotOutlined,
  UserOutlined,
  GithubOutlined,
  CopyOutlined,
} from '@ant-design/icons';
import {
  askStream,
  getRepo,
  SSEChunk,
  SSEDone,
  Citation,
  Repository,
} from '../api';
import SSEViewer from '../components/SSEViewer';
import Citations from '../components/Citations';

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  citations?: Citation[];
  confidence?: number;
  convId?: string;
}

export default function AskPage() {
  const { id: repoId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [question, setQuestion] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [currentChunk, setCurrentChunk] = useState('');
  const [citations, setCitations] = useState<Citation[]>([]);
  const [convId, setConvId] = useState<string | undefined>(undefined);
  const [repo, setRepo] = useState<Repository | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);

  // Fetch repo info
  useEffect(() => {
    if (!repoId) return;
    getRepo(repoId)
      .then(setRepo)
      .catch(() => message.error('Failed to load repository info'));
  }, [repoId]);

  // Auto-scroll to bottom
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, currentChunk]);

  const handleAsk = useCallback(() => {
    if (!question.trim() || !repoId) return;
    const q = question.trim();
    setQuestion('');
    setMessages((prev) => [...prev, { role: 'user', content: q }]);
    setCurrentChunk('');
    setCitations([]);
    setLoading(true);

    // Capture current citations in a ref-like pattern for the done callback
    let latestCitations: Citation[] = [];

    const controller = askStream(
      repoId,
      q,
      convId,
      (data: SSEChunk) => {
        setCurrentChunk((prev) => prev + (data.text || ''));
        if (data.citations?.length) {
          latestCitations = data.citations;
          setCitations(data.citations);
        }
      },
      (data: SSEDone) => {
        setMessages((prev) => [
          ...prev,
          {
            role: 'assistant',
            content: currentChunkRef.current,
            citations: [...latestCitations],
            confidence: data.confidence,
            convId: data.conv_id,
          },
        ]);
        setCurrentChunk('');
        setCitations([]);
        setLoading(false);
        if (data.conv_id && !convId) {
          setConvId(data.conv_id);
        }
      },
      (err: string) => {
        setMessages((prev) => [
          ...prev,
          { role: 'assistant', content: `Error: ${err}` },
        ]);
        setCurrentChunk('');
        setCitations([]);
        setLoading(false);
      },
    );
    controllerRef.current = controller;
  }, [question, repoId, convId]);

  // We need a ref to track the current chunk value for the closure in onDone
  const currentChunkRef = useRef('');
  useEffect(() => {
    currentChunkRef.current = currentChunk;
  }, [currentChunk]);

  const handleStop = () => {
    controllerRef.current?.abort();
    // Save what we have so far as a message
    if (currentChunk.trim()) {
      setMessages((prev) => [
        ...prev,
        {
          role: 'assistant',
          content: currentChunk,
          citations: [...citations],
        },
      ]);
    }
    setCurrentChunk('');
    setCitations([]);
    setLoading(false);
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      message.success('Copied to clipboard');
    });
  };

  if (!repoId) {
    return (
      <div style={{ padding: 24 }}>
        <Alert type="error" message="No repository specified" showIcon />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 64px)' }}>
      {/* Header bar */}
      <div
        style={{
          padding: '8px 16px',
          background: '#fff',
          borderBottom: '1px solid #f0f0f0',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Space>
          <Button
            icon={<ArrowLeftOutlined />}
            type="text"
            onClick={() => navigate(`/repos/${repoId}`)}
          >
            Back
          </Button>
          <Typography.Text strong>
            <GithubOutlined style={{ marginRight: 4 }} />
            {repo?.full_name || repoId}
          </Typography.Text>
          {repo && (
            <Tag color={repo.status === 'ready' ? 'green' : 'default'}>
              {repo.status}
            </Tag>
          )}
        </Space>
        <Space>
          {convId && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }} copyable>
              Conversation: {convId.substring(0, 8)}...
            </Typography.Text>
          )}
        </Space>
      </div>

      {/* Chat messages area */}
      <div
        style={{
          flex: 1,
          overflow: 'auto',
          padding: '16px 24px',
          background: '#fafafa',
        }}
      >
        {messages.length === 0 && !loading && (
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              opacity: 0.5,
            }}
          >
            <RobotOutlined style={{ fontSize: 48, marginBottom: 16 }} />
            <Typography.Title level={5} type="secondary">
              Ask questions about this codebase
            </Typography.Title>
            <Typography.Text type="secondary">
              Use AI to understand architecture, find bugs, and explore the code.
            </Typography.Text>
          </div>
        )}

        {messages.map((msg, i) => (
          <div
            key={i}
            style={{
              marginBottom: 16,
              display: 'flex',
              flexDirection: msg.role === 'user' ? 'row-reverse' : 'row',
            }}
          >
            <div
              style={{
                maxWidth: '80%',
                minWidth: 200,
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  marginBottom: 4,
                  flexDirection: msg.role === 'user' ? 'row-reverse' : 'row',
                }}
              >
                {msg.role === 'user' ? (
                  <UserOutlined style={{ color: '#1890ff' }} />
                ) : (
                  <RobotOutlined style={{ color: '#52c41a' }} />
                )}
                <Tag color={msg.role === 'user' ? 'blue' : 'green'}>
                  {msg.role === 'user' ? 'You' : 'Copilot'}
                </Tag>
                {msg.confidence != null && (
                  <Tag>
                    Confidence: {(msg.confidence * 100).toFixed(0)}%
                  </Tag>
                )}
              </div>
              <Card
                size="small"
                styles={{
                  body: {
                    padding: '12px 16px',
                    background: msg.role === 'user' ? '#e6f7ff' : '#fff',
                    borderRadius: 8,
                  },
                }}
                extra={
                  msg.role === 'assistant' && msg.content ? (
                    <Button
                      type="text"
                      size="small"
                      icon={<CopyOutlined />}
                      onClick={() => copyToClipboard(msg.content)}
                    />
                  ) : null
                }
              >
                <SSEViewer text={msg.content} />
                {msg.citations && msg.citations.length > 0 && (
                  <Citations items={msg.citations} />
                )}
              </Card>
            </div>
          </div>
        ))}

        {/* Streaming chunk */}
        {currentChunk && (
          <div style={{ marginBottom: 16, display: 'flex', flexDirection: 'row' }}>
            <div style={{ maxWidth: '80%', minWidth: 200 }}>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  marginBottom: 4,
                }}
              >
                <RobotOutlined style={{ color: '#52c41a' }} />
                <Tag color="green">Copilot</Tag>
                <Tag color="processing">streaming...</Tag>
              </div>
              <Card
                size="small"
                styles={{
                  body: {
                    padding: '12px 16px',
                    background: '#fff',
                    borderRadius: 8,
                  },
                }}
              >
                <SSEViewer text={currentChunk} />
                {citations.length > 0 && <Citations items={citations} />}
              </Card>
            </div>
          </div>
        )}

        {/* Loading indicator when waiting for first chunk */}
        {loading && !currentChunk && (
          <div style={{ marginBottom: 16, display: 'flex', flexDirection: 'row' }}>
            <div>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  marginBottom: 4,
                }}
              >
                <RobotOutlined style={{ color: '#52c41a' }} />
                <Tag color="green">Copilot</Tag>
              </div>
              <Card size="small">
                <Spin size="small" />{' '}
                <Typography.Text type="secondary">Thinking...</Typography.Text>
              </Card>
            </div>
          </div>
        )}

        <div ref={chatEndRef} />
      </div>

      {/* Input area */}
      <div
        style={{
          padding: '12px 24px',
          borderTop: '1px solid #f0f0f0',
          background: '#fff',
        }}
      >
        <Space.Compact style={{ width: '100%' }}>
          <Input.TextArea
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            onPressEnter={(e) => {
              if (!e.shiftKey) {
                e.preventDefault();
                handleAsk();
              }
            }}
            placeholder="Ask about this codebase... (Shift+Enter for new line)"
            rows={2}
            disabled={loading}
            autoSize={{ minRows: 2, maxRows: 6 }}
            style={{ resize: 'none' }}
          />
          {loading ? (
            <Button
              icon={<StopOutlined />}
              onClick={handleStop}
              danger
              style={{ height: 'auto' }}
            />
          ) : (
            <Button
              icon={<SendOutlined />}
              onClick={handleAsk}
              type="primary"
              disabled={!question.trim()}
              style={{ height: 'auto' }}
            />
          )}
        </Space.Compact>
      </div>
    </div>
  );
}
