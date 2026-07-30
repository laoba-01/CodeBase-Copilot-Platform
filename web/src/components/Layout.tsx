import React, { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Layout as AntLayout, Menu, Typography, Button, Space } from 'antd';
import {
  GithubOutlined,
  DatabaseOutlined,
  QuestionCircleOutlined,
  LogoutOutlined,
  LoginOutlined,
  GitlabOutlined,
} from '@ant-design/icons';

const { Header, Sider, Content } = AntLayout;

interface LayoutProps {
  children: React.ReactNode;
}

export default function Layout({ children }: LayoutProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const token = localStorage.getItem('token');

  // Capture token from GitHub OAuth redirect (?token=...&username=...)
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const urlToken = params.get('token');
    if (urlToken) {
      localStorage.setItem('token', urlToken);
      // Clean URL without reloading
      window.history.replaceState({}, '', '/');
      window.location.reload();
    }
  }, [location.search]);

  // Determine selected key from current path
  const pathParts = location.pathname.split('/').filter(Boolean);
  let selectedKey = '/';
  if (pathParts[0] === 'repos') {
    if (pathParts.length === 1 || (pathParts.length === 2 && pathParts[2] !== 'ask')) {
      selectedKey = '/repos';
    } else if (pathParts[2] === 'ask') {
      selectedKey = '/repos/:id/ask';
    }
  }

  const menuItems = [
    {
      key: '/',
      icon: <DatabaseOutlined />,
      label: 'Repositories',
    },
    ...(token
      ? [
          {
            key: 'ask-group',
            icon: <QuestionCircleOutlined />,
            label: 'Ask',
            disabled: !location.pathname.startsWith('/repos/'),
          },
        ]
      : []),
  ];

  const handleMenuClick = (info: { key: string }) => {
    if (info.key === '/') {
      navigate('/');
    }
  };

  const handleLogout = () => {
    localStorage.removeItem('token');
    window.location.href = '/';
  };

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 24px',
          background: '#001529',
        }}
      >
        <Space>
          <Typography.Title level={4} style={{ color: '#fff', margin: 0 }}>
            <GithubOutlined style={{ marginRight: 8 }} />
            Codebase Copilot
          </Typography.Title>
        </Space>
        <Space>
          {token ? (
            <Button
              type="text"
              icon={<LogoutOutlined />}
              onClick={handleLogout}
              style={{ color: '#fff' }}
            >
              Logout
            </Button>
          ) : (
            <Space size="small">
              <Button
                type="text"
                icon={<GithubOutlined />}
                onClick={() => {
                  const clientId = import.meta.env.VITE_GITHUB_CLIENT_ID;
                  if (clientId) {
                    window.location.href = `https://github.com/login/oauth/authorize?client_id=${clientId}&scope=repo`;
                  }
                }}
                style={{ color: '#fff' }}
              >
                GitHub
              </Button>
              <Button
                type="text"
                icon={<GitlabOutlined />}
                onClick={async () => {
                  try {
                    const res = await fetch('/auth/gitee/authorize');
                    const { url } = await res.json();
                    if (url) window.location.href = url;
                  } catch { /* ignore */ }
                }}
                style={{ color: '#fff' }}
              >
                Gitee
              </Button>
            </Space>
          )}
        </Space>
      </Header>
      <AntLayout>
        <Sider
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          width={200}
          style={{ background: '#fff' }}
        >
          <Menu
            mode="inline"
            selectedKeys={[selectedKey]}
            items={menuItems}
            onClick={handleMenuClick}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content
          style={{
            padding: 0,
            margin: 0,
            minHeight: 'calc(100vh - 64px)',
            background: '#f5f5f5',
          }}
        >
          {children}
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
