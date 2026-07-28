import React from 'react';
import { Card, Typography, Tag, Space, Tooltip } from 'antd';
import { FileOutlined, PercentageOutlined } from '@ant-design/icons';

export interface CitationItem {
  file: string;
  line: number;
  content?: string;
  score: number;
}

interface CitationsProps {
  items: CitationItem[];
}

const scoreColor = (score: number): string => {
  if (score >= 0.8) return 'green';
  if (score >= 0.5) return 'orange';
  return 'red';
};

export default function Citations({ items }: CitationsProps) {
  if (!items || items.length === 0) return null;

  return (
    <div style={{ marginTop: 12 }}>
      <Typography.Text type="secondary" strong style={{ fontSize: 12 }}>
        <FileOutlined style={{ marginRight: 4 }} />
        Sources ({items.length})
      </Typography.Text>
      <Space wrap style={{ marginTop: 4 }} size={[8, 8]}>
        {items.map((item, i) => (
          <Tooltip
            key={i}
            title={
              item.content
                ? `${item.content.substring(0, 200)}${item.content.length > 200 ? '...' : ''}`
                : `${item.file}:${item.line}`
            }
          >
            <Card
              size="small"
              styles={{
                body: { padding: '6px 10px' },
              }}
              style={{ cursor: 'default' }}
            >
              <Space size={6}>
                <FileOutlined style={{ fontSize: 12, color: '#1890ff' }} />
                <Typography.Text style={{ fontSize: 12 }}>
                  {item.file}:{item.line}
                </Typography.Text>
                <Tag
                  color={scoreColor(item.score)}
                  style={{ margin: 0, fontSize: 10, lineHeight: '16px' }}
                >
                  <PercentageOutlined style={{ fontSize: 10 }} />
                  {(item.score * 100).toFixed(0)}%
                </Tag>
              </Space>
            </Card>
          </Tooltip>
        ))}
      </Space>
    </div>
  );
}
