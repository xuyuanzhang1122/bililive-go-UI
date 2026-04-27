import React, { useState, useEffect, useCallback } from 'react';
import { List, Button, message, Empty, Popconfirm, Typography, Space, Tag } from 'antd';
import { DeleteOutlined, ClockCircleOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { authFetch } from '../../utils/common';

const { Text } = Typography;

interface HistoryEntry {
    id: number;
    video_path: string;
    video_name: string;
    position_seconds: number;
    duration_seconds: number;
    updated_at: string;
}

const formatTime = (seconds: number): string => {
    if (!seconds || seconds <= 0) return '00:00';
    const total = Math.floor(seconds);
    const h = Math.floor(total / 3600);
    const m = Math.floor((total % 3600) / 60);
    const s = total % 60;
    if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
};

const formatDate = (ts: string): string => {
    if (!ts) return '';
    try { return new Date(ts + 'Z').toLocaleString(); } catch { return ts; }
};

const HistoryPage: React.FC = () => {
    const navigate = useNavigate();
    const [entries, setEntries] = useState<HistoryEntry[]>([]);
    const [loading, setLoading] = useState(true);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const res = await authFetch('/api/history');
            if (res.ok) {
                const data = await res.json();
                setEntries(Array.isArray(data) ? data : []);
            }
        } catch { /* ignore */ }
        finally { setLoading(false); }
    }, []);

    useEffect(() => { load(); }, [load]);

    const deleteEntry = async (videoPath: string) => {
        try {
            const res = await authFetch('/api/history/' + encodeURIComponent(videoPath), { method: 'DELETE' });
            if (res.ok) {
                message.success('已删除');
                load();
            } else {
                message.error('删除失败');
            }
        } catch { message.error('删除失败'); }
    };

    const openVideo = (entry: HistoryEntry) => {
        const parts = entry.video_path.split('/');
        const folder = parts.slice(0, -1).join('/');
        const name = parts[parts.length - 1] || entry.video_name;
        const params = new URLSearchParams();
        if (folder) params.set('room', folder);
        params.set('play', entry.video_path);
        params.set('name', name || entry.video_name);
        navigate({ pathname: '/videoLibrary', search: params.toString() });
    };

    return (
        <div style={{ padding: 24, maxWidth: 800, margin: '0 auto' }}>
            <h2 style={{ marginBottom: 20 }}>观看历史</h2>
            <List
                loading={loading}
                dataSource={entries}
                locale={{ emptyText: <Empty description="暂无观看历史" /> }}
                renderItem={entry => (
                    <List.Item
                        actions={[
                            <Popconfirm
                                key="del"
                                title="删除这条记录？"
                                onConfirm={() => deleteEntry(entry.video_path)}
                            >
                                <Button size="small" icon={<DeleteOutlined />} danger type="text" />
                            </Popconfirm>,
                        ]}
                    >
                        <List.Item.Meta
                            title={
                                <Space>
                                    <Button
                                        type="link"
                                        style={{ padding: 0, textAlign: 'left' }}
                                        onClick={() => openVideo(entry)}
                                    >
                                        <PlayCircleOutlined style={{ marginRight: 6 }} />
                                        {entry.video_name || entry.video_path.split('/').pop() || '未知视频'}
                                    </Button>
                                </Space>
                            }
                            description={
                                <Space size="middle">
                                    <Tag color="blue">{formatTime(entry.position_seconds)} / {formatTime(entry.duration_seconds)}</Tag>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        <ClockCircleOutlined style={{ marginRight: 4 }} />
                                        {formatDate(entry.updated_at)}
                                    </Text>
                                </Space>
                            }
                        />
                    </List.Item>
                )}
            />
        </div>
    );
};

export default HistoryPage;
