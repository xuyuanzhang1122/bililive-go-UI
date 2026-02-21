import React, { useState, useEffect, useCallback } from 'react';
import { HashRouter as Router, Link, useLocation } from 'react-router-dom';
import { Layout, Menu, Button, Drawer } from 'antd';
import {
    MonitorOutlined,
    UnorderedListOutlined,
    DashboardOutlined,
    SettingOutlined,
    FolderOutlined,
    ToolOutlined,
    MenuFoldOutlined,
    MenuUnfoldOutlined,
    LineChartOutlined,
    CloudUploadOutlined,
    MenuOutlined,
    VideoCameraOutlined,
} from '@ant-design/icons';
import './layout.css';

const { Header, Content, Sider } = Layout;

// localStorage key 用于保存侧边栏收起状态
const SIDER_COLLAPSED_KEY = 'siderCollapsed';
// 移动端断点
const MOBILE_BREAKPOINT = 768;

interface Props {
    children?: React.ReactNode;
}

// 菜单项定义（视频库置顶，管理功能收入「高级」分组）
const useMenuItems = (onMenuClick?: () => void) => [
    {
        key: '/videoLibrary',
        icon: <VideoCameraOutlined />,
        label: <Link to="/videoLibrary" onClick={onMenuClick}>视频库</Link>,
    },
    {
        type: 'group' as const,
        label: <span style={{ fontSize: 11, color: '#bbb', userSelect: 'none' }}>— 高级 —</span>,
        children: [
            {
                key: '/liveList',
                icon: <MonitorOutlined />,
                label: <Link to="/liveList" onClick={onMenuClick}>监控列表</Link>,
            },
            {
                key: '/liveInfo',
                icon: <DashboardOutlined />,
                label: <Link to="/liveInfo" onClick={onMenuClick}>系统状态</Link>,
            },
            {
                key: '/configInfo',
                icon: <SettingOutlined />,
                label: <Link to="/configInfo" onClick={onMenuClick}>设置</Link>,
            },
            {
                key: '/fileList',
                icon: <FolderOutlined />,
                label: <Link to="/fileList" onClick={onMenuClick}>文件</Link>,
            },
            {
                key: 'tools',
                icon: <ToolOutlined />,
                label: <a href="/tools/" target="_blank" rel="noopener noreferrer" onClick={onMenuClick}>工具</a>,
            },
            {
                key: '/tasks',
                icon: <UnorderedListOutlined />,
                label: <Link to="/tasks" onClick={onMenuClick}>任务队列</Link>,
            },
            {
                key: '/iostats',
                icon: <LineChartOutlined />,
                label: <Link to="/iostats" onClick={onMenuClick}>IO 统计</Link>,
            },
            {
                key: '/update',
                icon: <CloudUploadOutlined />,
                label: <Link to="/update" onClick={onMenuClick}>更新</Link>,
            },
        ],
    },
];

// 根据当前路径计算选中的菜单 key
const getSelectedKey = (pathname: string): string => {
    if (pathname.startsWith('/fileList')) return '/fileList';
    if (pathname.startsWith('/videoLibrary')) return '/videoLibrary';
    if (pathname.startsWith('/liveInfo')) return '/liveInfo';
    if (pathname.startsWith('/configInfo')) return '/configInfo';
    if (pathname.startsWith('/tasks')) return '/tasks';
    if (pathname.startsWith('/iostats')) return '/iostats';
    if (pathname.startsWith('/update')) return '/update';
    if (pathname.startsWith('/liveList')) return '/liveList';
    return '/videoLibrary';
};

// 内部布局组件（需要在 Router 内部才能用 useLocation）
const InnerLayout: React.FC<Props> = ({ children }) => {
    const location = useLocation();
    const selectedKey = getSelectedKey(location.pathname);

    // 检查是否为移动端
    const [isMobile, setIsMobile] = useState(window.innerWidth < MOBILE_BREAKPOINT);
    // 移动端抽屉开关
    const [drawerOpen, setDrawerOpen] = useState(false);
    // PC 端侧边栏折叠状态
    const [collapsed, setCollapsed] = useState(() => {
        try {
            const saved = localStorage.getItem(SIDER_COLLAPSED_KEY);
            return saved === 'true';
        } catch {
            return false;
        }
    });

    // 监听窗口大小变化
    useEffect(() => {
        const handleResize = () => {
            setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
        };
        window.addEventListener('resize', handleResize);
        return () => window.removeEventListener('resize', handleResize);
    }, []);

    // 关闭 Drawer
    const closeDrawer = useCallback(() => setDrawerOpen(false), []);

    // PC 端切换折叠
    const toggleCollapsed = () => {
        const next = !collapsed;
        setCollapsed(next);
        try {
            localStorage.setItem(SIDER_COLLAPSED_KEY, String(next));
        } catch {
            // ignore
        }
    };

    const menuItems = useMenuItems(isMobile ? closeDrawer : undefined);

    return (
        <Layout className="all-layout">
            <Header className="header small-header">
                {/* 移动端汉堡按钮 */}
                {isMobile && (
                    <Button
                        className="mobile-menu-btn"
                        type="text"
                        icon={<MenuOutlined />}
                        onClick={() => setDrawerOpen(true)}
                    />
                )}
                <h3 className="logo-text">Bililive-go</h3>
            </Header>
            <Layout>
                {/* PC 端侧边栏 */}
                {!isMobile && (
                    <Sider
                        className="side-bar"
                        width={200}
                        collapsedWidth={60}
                        style={{ background: '#fff' }}
                        trigger={null}
                        collapsible
                        collapsed={collapsed}
                    >
                        <div style={{
                            padding: '12px 0',
                            borderBottom: '1px solid #f0f0f0',
                            width: '100%'
                        }}>
                            <Button
                                type="text"
                                icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                                onClick={toggleCollapsed}
                                style={{
                                    fontSize: 16,
                                    width: '100%',
                                    textAlign: 'left',
                                    paddingLeft: collapsed ? 20 : 24,
                                    height: 40
                                }}
                            >
                                {!collapsed && '收起菜单'}
                            </Button>
                        </div>
                        <Menu
                            mode="inline"
                            selectedKeys={[selectedKey]}
                            inlineCollapsed={collapsed}
                            style={{ borderRight: 0 }}
                            items={menuItems}
                        />
                    </Sider>
                )}

                {/* 移动端 Drawer 抽屉菜单 */}
                {isMobile && (
                    <Drawer
                        title="Bililive-go"
                        placement="left"
                        open={drawerOpen}
                        onClose={closeDrawer}
                        width={220}
                        styles={{ body: { padding: 0 } }}
                    >
                        <Menu
                            mode="inline"
                            selectedKeys={[selectedKey]}
                            style={{ borderRight: 0, height: '100%' }}
                            items={menuItems}
                        />
                    </Drawer>
                )}

                <Layout className="content-padding">
                    <Content
                        className="inside-content-padding"
                        style={{
                            background: '#fff',
                            margin: 0,
                            minHeight: 280,
                            overflow: 'auto',
                        }}
                    >
                        {children}
                    </Content>
                </Layout>
            </Layout>
        </Layout>
    );
};

// 外层组件提供 Router 上下文
const RootLayout: React.FC<Props> = ({ children }) => {
    return (
        <Router>
            <InnerLayout>{children}</InnerLayout>
        </Router>
    );
};

export default RootLayout;
