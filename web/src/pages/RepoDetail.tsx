import React, { useState, useEffect, useCallback } from 'react';
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
} from 'antd';
import {
  ArrowLeftOutlined,
  QuestionCircleOutlined,
  GithubOutlined,
  FolderOutlined,
  FileOutlined,
  ReloadOutlined,
  RedoOutlined,
} from '@ant-design/icons';
import type { DataNode } from 'antd/es/tree';
import { getRepo, getRepoFiles, reindexRepo, FileEntry, Repository } from '../api';

const statusColor: Record<string, string> = {
  pending: 'default',
  cloning: 'processing',
  indexing: 'processing',
  ready: 'green',
  error: 'red',
};

function buildFileTree(files: FileEntry[]): DataNode[] {
  const tree: DataNode[] = [];

  // Find or create a node at a given level of the tree
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
        // File node
        currentNodes.push({
          title: part,
          key: currentPath,
          isLeaf: true,
          icon: <FileOutlined />,
        });
      } else {
        // Directory node
        const dir = findOrCreateDir(currentNodes, part, currentPath);
        currentNodes = dir.children as DataNode[];
      }
    }
  }

  // Sort each level: directories first, then alphabetically
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
    } catch (err: any) {
      message.error(err.message || 'Failed to reindex');
    } finally {
      setReindexing(false);
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
        {/* Header with back button */}
        <Space>
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate('/')}
            type="text"
          >
            Back
          </Button>
          <Typography.Title level={4} style={{ margin: 0 }}>
            <GithubOutlined style={{ marginRight: 8 }} />
            {repo.full_name}
          </Typography.Title>
        </Space>

        {/* Repo info card */}
        <Card title="Repository Information">
          <Descriptions column={{ xs: 1, sm: 2 }} size="small" bordered>
            <Descriptions.Item label="Full Name">{repo.full_name}</Descriptions.Item>
            <Descriptions.Item label="Default Branch">
              {repo.default_branch || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="Status">
              <Tag color={statusColor[repo.status] || 'default'}>
                {repo.status}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Indexed At">
              {repo.indexed_at ? new Date(repo.indexed_at).toLocaleString() : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="Clone URL">
              <Typography.Text copyable style={{ fontSize: 12 }}>
                {repo.clone_url}
              </Typography.Text>
            </Descriptions.Item>
            <Descriptions.Item label="Created">
              {new Date(repo.created_at).toLocaleString()}
            </Descriptions.Item>
          </Descriptions>
        </Card>

        {/* Processing status */}
        {isProcessing && (
          <Card title="Indexing in Progress">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Progress
                percent={50}
                status="active"
                strokeColor={{ from: '#108ee9', to: '#87d068' }}
              />
              <Typography.Text type="secondary">
                The repository is being processed. This may take a few minutes depending on the size.
                The page will update automatically when ready.
              </Typography.Text>
            </Space>
          </Card>
        )}

        {/* File tree */}
        <Card
          title={
            <Space>
              <FolderOutlined />
              <span>File Tree</span>
            </Space>
          }
          extra={
            <Space>
              {repo.status === 'ready' && (
                <>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {fileTree.length > 0 ? `${fileTree.length} top-level items` : ''}
                  </Typography.Text>
                  <Button
                    icon={<RedoOutlined />}
                    onClick={handleReindex}
                    loading={reindexing}
                    size="small"
                  >
                    Reindex
                  </Button>
                </>
              )}
            </Space>
          }
        >
          {repo.status === 'ready' ? (
            filesLoading ? (
              <div style={{ textAlign: 'center', padding: 20 }}>
                <Spin tip="Loading files..." />
              </div>
            ) : fileTree.length > 0 ? (
              <Tree
                showIcon
                defaultExpandAll
                treeData={fileTree}
                icon={({ isLeaf }) =>
                  isLeaf ? <FileOutlined /> : <FolderOutlined />
                }
              />
            ) : (
              <Empty
                description="No indexed files found. Try reindexing."
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            )
          ) : (
            <Empty
              description="Repository must be indexed before the file tree is available."
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          )}
        </Card>

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
          {repo.status !== 'ready' && (
            <Typography.Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
              Q&A is available once the repository has been indexed.
            </Typography.Text>
          )}
        </div>
      </Space>
    </div>
  );
}
