import { Modal, Input, Button, Alert, Spin } from 'antd';
import React from 'react';
import API from '../../utils/api';

const api = new API();
const { TextArea } = Input;

interface Props {
    refresh?: any;
    children?: React.ReactNode;
}

// 从文本中提取 URL
function extractUrls(text: string): string[] {
    const urlRegex = /https?:\/\/[^\s\u3000\uff0c，。！？；：'"【】（）<>]+/g;
    return Array.from(new Set(text.match(urlRegex) || []));
}

// 是否为抖音短链
function isDouyinShortUrl(url: string): boolean {
    return /v\.douyin\.com|iesdouyin\.com/i.test(url);
}

// 是否为直播间 URL（直接可用）
function isLiveRoomUrl(url: string): boolean {
    return /live\.(douyin|bilibili|huya|douyu|twitch|kuaishou)\.com/i.test(url)
        || /cc\.163\.com|live\.acfun\.cn|www\.huajiao\.com|fm\.missevan\.com|www\.yy\.com|weibo\.com|webcast\.(amemv|douyin)\.com/i.test(url)
        || /live\./i.test(url);
}

function normalizeRoomUrl(raw: string): string {
    const v = raw.trim();
    if (!v) return '';
    try {
        const u = new URL(v);
        const host = u.hostname.toLowerCase();
        if (host.includes('douyin.com') || host.includes('amemv.com')) {
            let roomId = '';
            const pathSegs = u.pathname.split('/').filter(Boolean);
            for (const seg of pathSegs) {
                if (/^\d{6,}$/.test(seg)) roomId = seg;
            }
            if (!roomId) {
                for (const k of ['room_id', 'web_rid', 'roomId']) {
                    const candidate = u.searchParams.get(k) || '';
                    if (/^\d{6,}$/.test(candidate)) { roomId = candidate; break; }
                }
            }
            if (roomId) return `https://live.douyin.com/${roomId}`;
        }
        return u.toString();
    } catch {
        return v;
    }
}

class AddRoomDialog extends React.Component<Props> {
    state = {
        visible: false,
        confirmLoading: false,
        // 用户输入的原始文本
        inputText: '',
        // 解析状态
        resolving: false,
        resolvedUrl: '',
        resolveError: '',
        // 识别到的候选 URL（可能有多个）
        candidateUrls: [] as string[],
    };

    showModal = () => {
        this.setState({
            visible: true,
            confirmLoading: false,
            inputText: '',
            resolving: false,
            resolvedUrl: '',
            resolveError: '',
            candidateUrls: [],
        });
    };

    // 提取并解析 URL
    handleTextChange = async (text: string) => {
        this.setState({ inputText: text, resolvedUrl: '', resolveError: '', candidateUrls: [] });
        if (!text.trim()) return;

        const rawUrls = extractUrls(text);
        if (rawUrls.length === 0) return;
        const urls = Array.from(new Set(rawUrls.map(normalizeRoomUrl).filter(Boolean)));

        // 优先处理抖音短链
        const shortUrl = rawUrls.find(isDouyinShortUrl);
        const directUrl = urls.find(isLiveRoomUrl);

        if (shortUrl) {
            this.setState({ resolving: true, resolveError: '' });
            try {
                const result = await api.resolveUrl(shortUrl) as { url: string };
                this.setState({ resolving: false, resolvedUrl: normalizeRoomUrl(result.url), candidateUrls: urls });
            } catch (e) {
                this.setState({ resolving: false, resolveError: '短链解析失败，请手动输入标准地址', candidateUrls: urls });
            }
        } else if (directUrl) {
            this.setState({ resolvedUrl: directUrl, candidateUrls: urls });
        } else if (urls.length > 0) {
            // 有 URL 但不确定，列出候选
            this.setState({ candidateUrls: urls });
        }
    };

    // 获取最终要提交的 URL
    getFinalUrl = (): string => {
        const { resolvedUrl, candidateUrls, inputText } = this.state;
        if (resolvedUrl) return normalizeRoomUrl(resolvedUrl);
        if (candidateUrls.length === 1) return normalizeRoomUrl(candidateUrls[0]);
        // 如果输入的就是一个标准 URL，直接使用
        const trimmed = inputText.trim();
        if (trimmed.startsWith('http')) return normalizeRoomUrl(trimmed);
        return '';
    };

    handleOk = () => {
        const url = this.getFinalUrl();
        if (!url) {
            this.setState({ resolveError: '请输入有效的直播间地址' });
            return;
        }
        this.setState({ confirmLoading: true });
        api.addNewRoom(url)
            .then(() => {
                api.saveSettingsInBackground();
                this.setState({ visible: false, confirmLoading: false, inputText: '' });
                this.props.refresh?.();
            })
            .catch(err => {
                alert(`添加直播间失败:\n${err}`);
                this.setState({ confirmLoading: false });
            });
    };

    handleCancel = () => {
        this.setState({ visible: false, inputText: '' });
    };

    render() {
        const { visible, confirmLoading, inputText, resolving, resolvedUrl, resolveError, candidateUrls } = this.state;
        const finalUrl = this.getFinalUrl();

        return (
            <div>
                <Modal
                    title="添加直播间"
                    open={visible}
                    onOk={this.handleOk}
                    confirmLoading={confirmLoading}
                    onCancel={this.handleCancel}
                    okText="添加"
                    cancelText="取消"
                    okButtonProps={{ disabled: !finalUrl && !resolving }}
                >
                    <div style={{ marginBottom: 8, color: '#666', fontSize: 13 }}>
                        支持粘贴抖音/哔哩哔哩等平台的分享链接或直播间地址
                    </div>
                    <TextArea
                        rows={4}
                        value={inputText}
                        placeholder={`可直接粘贴分享文案，例如：\n"【主播名】正在直播，https://v.douyin.com/xxx/"`}
                        onChange={e => this.handleTextChange(e.target.value)}
                        autoFocus
                    />
                    {/* 解析中 */}
                    {resolving && (
                        <div style={{ marginTop: 8, display: 'flex', alignItems: 'center', gap: 8, color: '#1890ff' }}>
                            <Spin size="small" />
                            <span>正在解析短链...</span>
                        </div>
                    )}
                    {/* 解析成功 */}
                    {resolvedUrl && !resolving && (
                        <Alert
                            style={{ marginTop: 8 }}
                            type="success"
                            message={
                                <span>
                                    已识别直播间地址：
                                    <a href={resolvedUrl} target="_blank" rel="noopener noreferrer"
                                        style={{ wordBreak: 'break-all', fontSize: 12 }}>
                                        {resolvedUrl}
                                    </a>
                                </span>
                            }
                        />
                    )}
                    {/* 有候选但未解析成功，显示列表 */}
                    {!resolvedUrl && !resolving && candidateUrls.length > 1 && (
                        <div style={{ marginTop: 8 }}>
                            <div style={{ color: '#666', fontSize: 12, marginBottom: 4 }}>识别到多个链接，请选择：</div>
                            {candidateUrls.map(url => (
                                <Button
                                    key={url}
                                    type="link"
                                    size="small"
                                    style={{ display: 'block', textAlign: 'left', height: 'auto', padding: '2px 0', fontSize: 12 }}
                                    onClick={() => this.setState({ resolvedUrl: url })}
                                >
                                    {url}
                                </Button>
                            ))}
                        </div>
                    )}
                    {/* 解析失败 */}
                    {resolveError && (
                        <Alert style={{ marginTop: 8 }} type="warning" message={resolveError} />
                    )}
                    {/* 没有自动识别，显示手动输入提示 */}
                    {!resolving && !resolvedUrl && !resolveError && candidateUrls.length === 0 && inputText.trim() && (
                        <Alert
                            style={{ marginTop: 8 }}
                            type="info"
                            message="未识别到直播间链接，请确认输入了完整的直播间地址"
                        />
                    )}
                </Modal>
            </div>
        );
    }
}

export default AddRoomDialog;
