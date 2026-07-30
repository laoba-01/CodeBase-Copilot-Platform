import { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Descriptions,
  Button,
  Tag,
  Typography,
  Spin,
  Space,
  Tree,
  Empty,
  message,
  Progress,
  Alert,
  Row,
  Col,
} from 'antd';
import {
  ArrowLeftOutlined,
  QuestionCircleOutlined,
  GithubOutlined,
  FolderOutlined,
  FileOutlined,
  ReloadOutlined,
  RedoOutlined,
  CodeOutlined,
  FunctionOutlined,
} from '@ant-design/icons';
import type { DataNode } from 'antd/es/tree';
import { getRepo, getRepoFiles, getFileContent, reindexRepo, FileEntry, FileContentNode, Repository } from '../api';

const statusColor: Record<string, string> = {
  pending: 'default',
  cloning: 'processing',
  indexing: 'processing',
  ready: 'green',
  error: 'red',
};

function buildFileTree(files: FileEntry[]): DataNode[] {
  const tree: DataNode[] = [];

  function findOrCreateDir(nodes: DataNode[], name: string, fullPath: string): DataNode {
    let node = nodes.find(n => n.key === fullPath);
    if (!node) {
      node = {
        title: name,
        key: fullPath,
        isLeaf: false,
        icon: <FolderOutlined />,
        children: [],
      };
      nodes.push(node);
    }
    return node;
  }

  for (const file of files) {
    const parts = file.file_path.split('/');
    let currentNodes = tree;
    let currentPath = '';

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      currentPath = currentPath ? `${currentPath}/${part}` : part;
      const isLast = i === parts.length - 1;

      if (isLast) {
        currentNodes.push({
          title: part,
          key: currentPath,
          isLeaf: true,
          icon: <FileOutlined />,
        });
      } else {
        const dir = findOrCreateDir(currentNodes, part, currentPath);
        currentNodes = dir.children as DataNode[];
      }
    }
  }

  function sortTree(nodes: DataNode[]) {
    nodes.sort((a, b) => {
      const aLeaf = a.isLeaf ? 1 : 0;
      const bLeaf = b.isLeaf ? 1 : 0;
      if (aLeaf !== bLeaf) return aLeaf - bLeaf;
      return (a.title as string).localeCompare(b.title as string);
    });
    for (const node of nodes) {
      if (node.children && node.children.length > 0) {
        sortTree(node.children as DataNode[]);
      }
    }
  }
  sortTree(tree);

  return tree;
}

export default function RepoDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [repo, setRepo] = useState<Repository | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [fileTree, setFileTree] = useState<DataNode[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const [reindexing, setReindexing] = useState(false);

  // Code viewer state
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [codeNodes, setCodeNodes] = useState<FileContentNode[] | null>(null);
  const [codeLoading, setCodeLoading] = useState(false);

  const fetchRepo = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const data = await getRepo(id);
      setRepo(data);
    } catch (err: any) {
      setError(err.message || 'Failed to load repository');
    } finally {
      setLoading(false);
    }
  }, [id]);

  const fetchFiles = useCallback(async () => {
    if (!id) return;
    setFilesLoading(true);
    try {
      const files = await getRepoFiles(id);
      const tree = buildFileTree(files);
      setFileTree(tree);
    } catch {
      // silently ignore file fetch errors
    } finally {
      setFilesLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchRepo();
  }, [fetchRepo]);

  useEffect(() => {
    if (repo?.status === 'ready') {
      fetchFiles();
    }
  }, [repo?.status, fetchFiles]);

  // Poll for task status if repo is indexing
  useEffect(() => {
    if (!repo || repo.status === 'ready' || repo.status === 'error') return;

    const pollInterval = setInterval(async () => {
      try {
        const updated = await getRepo(id!);
        setRepo(updated);
        if (updated.status === 'ready' || updated.status === 'error') {
          clearInterval(pollInterval);
        }
      } catch {
        // ignore poll errors
      }
    }, 3000);

    return () => clearInterval(pollInterval);
  }, [repo?.status, id]);

  const handleReindex = async () => {
    if (!id) return;
    setReindexing(true);
    try {
      await reindexRepo(id);
      message.success('Reindex started');
      setRepo(prev => prev ? { ...prev, status: 'pending' } : null);
      setSelectedFile(null);
      setCodeNodes(null);
    } catch (err: any) {
      message.error(err.message || 'Failed to reindex');
    } finally {
      setReindexing(false);
    }
  };

  const handleFileSelect = async (selectedKeys: React.Key[]) => {
    if (selectedKeys.length === 0) return;
    const path = selectedKeys[0] as string;
    // Only fetch for leaf nodes (files), not directories
    setSelectedFile(path);
    setCodeLoading(true);
    setCodeNodes(null);
    try {
      const nodes = await getFileContent(id!, path);
      setCodeNodes(nodes);
    } catch {
      setCodeNodes([]);
    } finally {
      setCodeLoading(false);
    }
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '60vh' }}>
        <Spin size="large" tip="Loading repository..." />
      </div>
    );
  }

  if (error || !repo) {
    return (
      <div style={{ padding: 24 }}>
        <Alert
          type="error"
          message="Error loading repository"
          description={error || 'Repository not found'}
          showIcon
          action={
            <Space>
              <Button onClick={() => navigate('/')}>Back to List</Button>
              <Button onClick={fetchRepo} icon={<ReloadOutlined />}>
                Retry
              </Button>
            </Space>
          }
        />
      </div>
    );
  }

  const isProcessing = repo.status === 'cloning' || repo.status === 'indexing' || repo.status === 'pending';

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {/* Header */}
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')} type="text">Back</Button>
          <Typography.Title level={4} style={{ margin: 0 }}>
            <GithubOutlined style={{ marginRight: 8 }} />{repo.full_name}
          </Typography.Title>
        </Space>

        {/* Repo info */}
        <Card title="Repository Information" size="small">
          <Descriptions column={{ xs: 1, sm: 3 }} size="small" bordered>
            <Descriptions.Item label="Status">
              <Tag color={statusColor[repo.status] || 'default'}>{repo.status}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Branch">{repo.default_branch || '-'}</Descriptions.Item>
            <Descriptions.Item label="Indexed">
              {repo.indexed_at ? new Date(repo.indexed_at).toLocaleString() : '-'}
            </Descriptions.Item>
          </Descriptions>
        </Card>

        {/* Processing status */}
        {isProcessing && (
          <Card title="Indexing in Progress" size="small">
            <Progress percent={50} status="active" />
            <Typography.Text type="secondary">
              Processing repository... This may take several minutes. Page auto-updates.
            </Typography.Text>
          </Card>
        )}

        {/* Main content: File tree + Code viewer */}
        <Row gutter={16}>
          {/* File tree */}
          <Col xs={24} md={8}>
            <Card
              title={<Space><FolderOutlined /><span>Files</span></Space>}
              size="small"
              extra={
                repo.status === 'ready' && (
                  <Button icon={<RedoOutlined />} onClick={handleReindex} loading={reindexing} size="small">Reindex</Button>
                )
              }
              style={{ height: 'calc(100vh - 320px)', overflow: 'auto' }}
            >
              {repo.status === 'ready' ? (
                filesLoading ? (
                  <Spin tip="Loading..." />
                ) : fileTree.length > 0 ? (
                  <Tree
                    showIcon
                    treeData={fileTree}
                    onSelect={handleFileSelect}
                    selectedKeys={selectedFile ? [selectedFile] : []}
                    icon={({ isLeaf }) => isLeaf ? <FileOutlined /> : <FolderOutlined />}
                  />
                ) : (
                  <Empty description="No files indexed" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )
              ) : (
                <Empty description="Indexing required" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              )}
            </Card>
          </Col>

          {/* Code viewer */}
          <Col xs={24} md={16}>
            <Card
              title={
                <Space>
                  <CodeOutlined />
                  <span>{selectedFile || 'Select a file to view code'}</span>
                </Space>
              }
              size="small"
              style={{ height: 'calc(100vh - 320px)', overflow: 'auto' }}
              bodyStyle={{ padding: 0 }}
            >
              {!selectedFile ? (
                <div style={{ padding: 60, textAlign: 'center', opacity: 0.4 }}>
                  <CodeOutlined style={{ fontSize: 48, marginBottom: 16 }} />
                  <Typography.Text type="secondary">Click a file in the tree to view its code</Typography.Text>
                </div>
              ) : codeLoading ? (
                <div style={{ padding: 40, textAlign: 'center' }}><Spin /></div>
              ) : codeNodes && codeNodes.length === 0 ? (
                <div style={{ padding: 40, textAlign: 'center', opacity: 0.5 }}>
                  <Typography.Text type="secondary">No indexed symbols found in this file</Typography.Text>
                </div>
              ) : (
                <div style={{ padding: '8px 0' }}>
                  {codeNodes?.map((node) => (
                    <Card
                      key={node.id}
                      size="small"
                      type="inner"
                      style={{ margin: '0 8px 8px 8px' }}
                      title={
                        <Space size="small">
                          <FunctionOutlined style={{ color: '#1890ff' }} />
                          <Typography.Text code strong>{node.name}</Typography.Text>
                          <Tag color="blue">{node.type}</Tag>
                          <Tag>{node.language}</Tag>
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            L{node.start_line}-{node.end_line}
                          </Typography.Text>
                        </Space>
                      }
                    >
                      {node.signature && (
                        <Typography.Paragraph style={{ marginBottom: 8 }}>
                          <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                            {node.signature}
                          </Typography.Text>
                        </Typography.Paragraph>
                      )}
                      {node.code && (
                        <pre style={{
                          background: '#1e1e1e',
                          color: '#d4d4d4',
                          padding: '12px 16px',
                          borderRadius: 6,
                          fontSize: 13,
                          lineHeight: 1.6,
                          overflow: 'auto',
                          maxHeight: 400,
                          margin: 0,
                          fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', Consolas, monospace",
                        }}>
                          <code>{node.code}</code>
                        </pre>
                      )}
                    </Card>
                  ))}
                </div>
              )}
            </Card>
          </Col>
        </Row>

        {/* Ask button */}
        <div style={{ textAlign: 'center' }}>
          <Button
            type="primary"
            size="large"
            icon={<QuestionCircleOutlined />}
            disabled={repo.status !== 'ready'}
            onClick={() => navigate(`/repos/${repo.id}/ask`)}
          >
            Ask About This Codebase
          </Button>
        </div>
      </Space>
    </div>
  );
}
