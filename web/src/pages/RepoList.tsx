import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Card,
  Table,
  Button,
  Modal,
  Input,
  Tag,
  Space,
  Typography,
  message,
  Popconfirm,
  Empty,
  Spin,
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  QuestionCircleOutlined,
  ReloadOutlined,
  GithubOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { listRepos, createRepo, deleteRepo, Repository } from '../api';

const statusColor: Record<string, string> = {
  pending: 'default',
  cloning: 'processing',
  indexing: 'processing',
  ready: 'green',
  error: 'red',
};

export default function RepoList() {
  const navigate = useNavigate();
  const [repos, setRepos] = useState<Repository[]>([]);
  const [loading, setLoading] = useState(false);
  const [addModalOpen, setAddModalOpen] = useState(false);
  const [newRepoName, setNewRepoName] = useState('');
  const [adding, setAdding] = useState(false);
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);

  const fetchRepos = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listRepos();
      setRepos(data);
    } catch (err: any) {
      message.error(err.message || 'Failed to load repositories');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRepos();
  }, [fetchRepos]);

  const handleAdd = async () => {
    if (!newRepoName.trim()) return;
    setAdding(true);
    try {
      await createRepo(newRepoName.trim());
      message.success('Repository added successfully');
      setNewRepoName('');
      setAddModalOpen(false);
      fetchRepos();
    } catch (err: any) {
      message.error(err.message || 'Failed to add repository');
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteRepo(id);
      message.success('Repository deleted');
      fetchRepos();
    } catch (err: any) {
      message.error(err.message || 'Failed to delete repository');
    }
  };

  const columns: ColumnsType<Repository> = [
    {
      title: 'Repository',
      dataIndex: 'full_name',
      key: 'full_name',
      render: (name: string, record: Repository) => (
        <Space>
          <GithubOutlined />
          <a onClick={() => navigate(`/repos/${record.id}`)}>{name}</a>
        </Space>
      ),
    },
    {
      title: 'Branch',
      dataIndex: 'default_branch',
      key: 'default_branch',
      width: 120,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: string) => (
        <Tag color={statusColor[status] || 'default'}>{status}</Tag>
      ),
    },
    {
      title: 'Indexed',
      dataIndex: 'indexed_at',
      key: 'indexed_at',
      width: 180,
      render: (val: string | undefined) =>
        val ? new Date(val).toLocaleString() : '-',
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 160,
      render: (_: any, record: Repository) => (
        <Space>
          <Button
            size="small"
            icon={<QuestionCircleOutlined />}
            disabled={record.status !== 'ready'}
            onClick={() => navigate(`/repos/${record.id}/ask`)}
          >
            Ask
          </Button>
          <Popconfirm
            title="Delete this repository?"
            description="This will remove the repo and its index data."
            onConfirm={() => handleDelete(record.id)}
            okText="Delete"
            cancelText="Cancel"
            okButtonProps={{ danger: true }}
          >
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <Card
        title={
          <Space>
            <Typography.Title level={5} style={{ margin: 0 }}>
              Repositories
            </Typography.Title>
            <Typography.Text type="secondary">
              ({repos.length} total)
            </Typography.Text>
          </Space>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchRepos}>
              Refresh
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setAddModalOpen(true)}
            >
              Add Repository
            </Button>
          </Space>
        }
      >
        <Spin spinning={loading}>
          {repos.length === 0 && !loading ? (
            <Empty
              description="No repositories yet. Add one to get started."
              style={{ padding: 40 }}
            >
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => setAddModalOpen(true)}
              >
                Add Repository
              </Button>
            </Empty>
          ) : (
            <Table
              columns={columns}
              dataSource={repos}
              rowKey="id"
              pagination={false}
              size="middle"
            />
          )}
        </Spin>
      </Card>

      <Modal
        title="Add Repository"
        open={addModalOpen}
        onOk={handleAdd}
        onCancel={() => {
          setAddModalOpen(false);
          setNewRepoName('');
        }}
        confirmLoading={adding}
        okText="Add"
      >
        <Input
          placeholder="owner/repo (e.g. facebook/react)"
          value={newRepoName}
          onChange={(e) => setNewRepoName(e.target.value)}
          onPressEnter={handleAdd}
          prefix={<GithubOutlined />}
          size="large"
        />
      </Modal>
    </div>
  );
}
