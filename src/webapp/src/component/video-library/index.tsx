import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { Card, Row, Col, Spin, Empty, Tag, Typography, Tooltip, Button, Modal, Input, Alert, Space, message } from 'antd';
import {
    VideoCameraOutlined,
    FolderOpenOutlined,
    ClockCircleOutlined,
    PlusOutlined,
    ArrowLeftOutlined,
    PlayCircleOutlined,
} from '@ant-design/icons';
import Artplayer from 'artplayer';
import mpegtsjs from 'mpegts.js';
import { useNavigate, useSearchParams } from 'react-router-dom';
import API from '../../utils/api';
import Utils from '../../utils/common';
import './video-library.css';

const api = new API();
const { Text, Title } = Typography;

// ===== 播放历史记录（localStorage，每10秒保存） =====
const HISTORY_KEY = 'bililive_play_history';

interface PlayRecord {
    relPath: string;
    name: string;
    position: number;
    duration: number;
    savedAt: number;
}

function savePlayRecord(relPath: string, name: string, position: number, duration: number) {
    try {
        const record: PlayRecord = { relPath, name, position, duration, savedAt: Date.now() };
        localStorage.setItem(HISTORY_KEY, JSON.stringify(record));
        return record;
    } catch { }
    return null;
}

function loadPlayRecord(): PlayRecord | null {
    try {
        const s = localStorage.getItem(HISTORY_KEY);
        return s ? JSON.parse(s) : null;
    } catch { return null; }
}

// ===== 接口类型 =====
interface VideoRoomInfo {
    host_name: string;
    platform: string;
    folder_path: string;
    video_count: number;
    total_size: number;
    latest_video_at: number;
    latest_video: string;
}

interface VideoFileInfo {
    name: string;
    rel_path: string;
    size: number;
    mod_time: number;
}

// ===== 添加直播间弹窗 =====
function extractUrls(text: string): string[] {
    const reg = /https?:\/\/[^\s\u3000\uff0c，。！？；："'【】（）<>]+/g;
    return Array.from(new Set(text.match(reg) || []));
}
const isShortUrl = (u: string) => /v\.douyin\.com|iesdouyin\.com/i.test(u);
// webcast.amemv.com 不在列表内：bililive-go 无法解析该格式地址
const isLiveUrl = (u: string) => /live\.(douyin|bilibili|huya|douyu|twitch|kuaishou)\.com/i.test(u)
    || /cc\.163\.com|live\.acfun\.cn/.test(u);

function normalizeRoomUrl(raw: string): string {
    const v = raw.trim();
    if (!v) return '';
    try {
        const u = new URL(v);
        const host = u.hostname.toLowerCase();
        // 只处理 live.douyin.com（直接清理 query 保留纯海外地址）
        // webcast.amemv.com 的路径 ID 是 webcast_id，不是 live room_id，两者数字不同，不能直接转换。
        if (host === 'live.douyin.com') {
            const pathSegs = u.pathname.split('/').filter(Boolean);
            let roomId = '';
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
        } else if (host.includes('douyin.com') && !host.includes('webcast') && !host.includes('amemv')) {
            // v.douyin.com 等其他 douyin 子域名，尝试提取数字 ID
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

interface AddRoomModalProps { visible: boolean; onClose: () => void; onAdded: () => void; }
const AddRoomModal: React.FC<AddRoomModalProps> = ({ visible, onClose, onAdded }) => {
    const [text, setText] = useState('');
    const [resolving, setResolving] = useState(false);
    const [resolved, setResolved] = useState('');
    const [error, setError] = useState('');
    const [candidates, setCandidates] = useState<string[]>([]);
    const [adding, setAdding] = useState(false);

    const reset = () => { setText(''); setResolved(''); setError(''); setCandidates([]); };

    const handleChange = async (v: string) => {
        setText(v); setResolved(''); setError(''); setCandidates([]);
        const rawUrls = extractUrls(v);
        if (!rawUrls.length) return;
        const urls = Array.from(new Set(rawUrls.map(normalizeRoomUrl).filter(Boolean)));
        const short = rawUrls.find(isShortUrl);
        const direct = urls.find(isLiveUrl);
        if (short) {
            setResolving(true);
            try {
                const r = await api.resolveUrl(short) as { url: string };
                const nr = normalizeRoomUrl(r.url);
                // webcast.amemv.com URL 或超长数字（可能是 webcast stream ID 而非 room ID）
                const isWebcast = /webcast\.(amemv|douyin)\.com/i.test(nr);
                const isStreamId = /live\.douyin\.com\/\d{16,}$/.test(nr); // webcast ID 通常 18-19 位
                if (isWebcast || isStreamId) {
                    setError('短链已跳转至直播流地址，无法自动提取直播间号。请在抖音 App 中打开该主播直播间，复制地址栏 live.douyin.com/XXXXX 格式的地址后手动粘贴。');
                } else {
                    setResolved(nr);
                }
            } catch { setError('短链解析失败，请手动输入标准地址'); }
            setResolving(false);
            setCandidates(urls);
        } else if (direct) {
            setResolved(direct); setCandidates(urls);
        } else if (urls.length) {
            setCandidates(urls);
        }
    };

    const finalUrl = (): string => {
        if (resolved) return normalizeRoomUrl(resolved);
        if (candidates.length === 1) return normalizeRoomUrl(candidates[0]);
        const t = text.trim();
        if (!t.startsWith('http')) return '';
        return normalizeRoomUrl(t);
    };

    const handleOk = () => {
        const url = finalUrl();
        if (!url) { setError('请输入有效的直播间地址'); return; }
        setAdding(true);
        api.addNewRoom(url)
            .then(() => {
                api.saveSettingsInBackground();
                message.success('直播间已添加，开始监控中。录制完成后将出现在视频库。');
                reset(); onClose(); onAdded();
            })
            .catch((e: any) => { alert(`添加失败:\n${e}`); })
            .finally(() => setAdding(false));
    };

    return (
        <Modal
            title={<><PlusOutlined style={{ marginRight: 8 }} />添加直播间</>}
            open={visible}
            onOk={handleOk}
            confirmLoading={adding}
            onCancel={() => { reset(); onClose(); }}
            okText="添加" cancelText="取消"
            okButtonProps={{ disabled: !finalUrl() || resolving }}
        >
            <p style={{ color: '#888', fontSize: 13, margin: '0 0 8px' }}>支持粘贴抖音/B站等平台分享文案或直播间地址</p>
            <Input.TextArea
                rows={3} value={text}
                placeholder={`直接粘贴分享文案即可，例如:\n"主播正在直播 https://v.douyin.com/xxx/"`}
                onChange={e => handleChange(e.target.value)} autoFocus
            />
            {resolving && <div style={{ marginTop: 8, color: '#1890ff' }}><Spin size="small" /> 正在解析短链...</div>}
            {resolved && !resolving && <Alert style={{ marginTop: 8 }} type="success" message={<span>识别地址：<a href={resolved} target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>{resolved}</a></span>} />}
            {!resolved && !resolving && candidates.length > 1 && candidates.map(u => (
                <Button key={u} type="link" size="small" style={{ display: 'block', textAlign: 'left', height: 'auto', padding: '2px 0', fontSize: 12 }} onClick={() => setResolved(u)}>{u}</Button>
            ))}
            {error && <Alert style={{ marginTop: 8 }} type="warning" message={error} />}
        </Modal>
    );
};

// ===== 手势提示图标 SVG =====
const IconPlay = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="48" height="48">
        <path d="M8 5v14l11-7z" />
    </svg>
);
const IconPause = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="48" height="48">
        <path d="M6 19h4V5H6zm8-14v14h4V5z" />
    </svg>
);
const IconSeekForward = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="48" height="48">
        <path d="M4 18l8.5-6L4 6v12zm9-12v12l8.5-6L13 6z" />
    </svg>
);
const IconSeekBack = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="48" height="48">
        <path d="M11 18V6l-8.5 6 8.5 6zm.5-6 8.5 6V6l-8.5 6z" />
    </svg>
);
const IconSpeed = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="48" height="48">
        <path d="M21 3L3 10.53v.98l6.84 2.65L12.48 21h.98L21 3z" />
    </svg>
);
const IconBack = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
        <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z" />
    </svg>
);
const IconControlPlay = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
        <path d="M8 5v14l11-7z" />
    </svg>
);
const IconControlPause = () => (
    <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
        <path d="M6 19h4V5H6zm8-14v14h4V5z" />
    </svg>
);
const IconBackward10 = () => (
    <svg viewBox="0 0 24 24" fill="none" width="20" height="20" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M11 19L2.5 12 11 5" />
        <path d="M20 19V5" />
        <path d="M14 10h2v4" />
        <path d="M14 14h3" />
    </svg>
);
const IconForward10 = () => (
    <svg viewBox="0 0 24 24" fill="none" width="20" height="20" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M13 5l8.5 7-8.5 7" />
        <path d="M4 5v14" />
        <path d="M8 10h2v4" />
        <path d="M8 14h3" />
    </svg>
);
const IconFullscreen = () => (
    <svg viewBox="0 0 24 24" fill="none" width="20" height="20" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M8 3H3v5" />
        <path d="M16 3h5v5" />
        <path d="M21 16v5h-5" />
        <path d="M3 16v5h5" />
    </svg>
);
const IconFullscreenExit = () => (
    <svg viewBox="0 0 24 24" fill="none" width="20" height="20" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M9 9H4V4" />
        <path d="M15 9h5V4" />
        <path d="M15 15h5v5" />
        <path d="M9 15H4v5" />
    </svg>
);

// ===== 手势提示类型 =====
type GestureHint =
    | { type: 'play' }
    | { type: 'pause' }
    | { type: 'seek'; delta: number }
    | { type: 'speed' };

// ===== Artplayer 播放器（支持 FLV/TS/MP4，手势，播放记录）=====
interface VideoPlayerProps {
    file: VideoFileInfo;
    onBack: () => void;
    onRecordSaved?: (record: PlayRecord) => void;
}
const VideoPlayer: React.FC<VideoPlayerProps> = ({ file, onBack, onRecordSaved }) => {
    const pageRef = useRef<HTMLDivElement>(null);
    const overlayRef = useRef<HTMLDivElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const artRef = useRef<Artplayer | null>(null);
    const saveTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const uiTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const hintTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [hint, setHint] = useState<GestureHint | null>(null);
    const [toolbarVisible, setToolbarVisible] = useState(true);
    const toolbarTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [playing, setPlaying] = useState(false);
    const [duration, setDuration] = useState(0);
    const [currentTime, setCurrentTime] = useState(0);
    const [seekingBySlider, setSeekingBySlider] = useState(false);
    const seekingBySliderRef = useRef(false);
    const [sliderValue, setSliderValue] = useState(0);
    const [isFullscreen, setIsFullscreen] = useState(false);
    const streamTypeRef = useRef<'flv' | 'ts' | 'other'>('other');
    const mpegtsPlayerRef = useRef<any>(null);

    // 触摸状态
    const touch = useRef({
        startX: 0, startY: 0, startTime: 0,
        lastTapTime: 0,
        longPressTimer: null as ReturnType<typeof setTimeout> | null,
        singleTapTimer: null as ReturnType<typeof setTimeout> | null,
        isSeeking: false, seekDelta: 0,
        isLongPress: false,
    });

    const playUrl = `/files/${file.rel_path.split('/').map(encodeURIComponent).join('/')}`;
    // 仅在切换文件时读取一次续播位置，避免父组件重渲染触发播放器重建
    const resumeTime = useMemo(() => {
        const savedRecord = loadPlayRecord();
        return (savedRecord?.relPath === file.rel_path) ? savedRecord.position : 0;
    }, [file.rel_path]);

    const showHint = (h: GestureHint, duration = 1200) => {
        setHint(h);
        if (hintTimerRef.current) clearTimeout(hintTimerRef.current);
        hintTimerRef.current = setTimeout(() => setHint(null), duration);
    };

    // 工具栏自动隐藏
    const resetToolbarTimer = () => {
        setToolbarVisible(true);
        if (toolbarTimerRef.current) clearTimeout(toolbarTimerRef.current);
        toolbarTimerRef.current = setTimeout(() => setToolbarVisible(false), 4000);
    };

    const toggleToolbar = () => {
        setToolbarVisible(v => {
            if (v) {
                if (toolbarTimerRef.current) clearTimeout(toolbarTimerRef.current);
                return false;
            } else {
                if (toolbarTimerRef.current) clearTimeout(toolbarTimerRef.current);
                toolbarTimerRef.current = setTimeout(() => setToolbarVisible(false), 4000);
                return true;
            }
        });
    };

    useEffect(() => {
        resetToolbarTimer();
        const touchState = touch.current;
        return () => {
            if (toolbarTimerRef.current) clearTimeout(toolbarTimerRef.current);
            if (hintTimerRef.current) clearTimeout(hintTimerRef.current);
            if (touchState.singleTapTimer) clearTimeout(touchState.singleTapTimer);
            if (uiTimerRef.current) clearInterval(uiTimerRef.current);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const handleTouchStart = (e: React.TouchEvent) => {
        // 必须在此调用 preventDefault，防止 iOS 系统级长按选择放大镜
        e.preventDefault();
        if (e.touches.length !== 1) return;
        const t = touch.current;
        const pt = e.touches[0];
        t.startX = pt.clientX; t.startY = pt.clientY;
        t.startTime = Date.now(); t.isSeeking = false; t.seekDelta = 0; t.isLongPress = false;
        t.longPressTimer = setTimeout(() => {
            t.isLongPress = true;
            if (artRef.current) {
                try { artRef.current.video.playbackRate = 2.0; } catch { }
            }
            showHint({ type: 'speed' }, 8000);
        }, 450);
    };

    const handleTouchMove = (e: React.TouchEvent) => {
        const t = touch.current;
        const pt = e.touches[0];
        const dx = pt.clientX - t.startX;
        const dy = pt.clientY - t.startY;
        if (Math.abs(dx) > 32 || Math.abs(dy) > 32) {
            if (t.longPressTimer) { clearTimeout(t.longPressTimer); t.longPressTimer = null; }
        }
        if (!t.isLongPress && Math.abs(dx) > 20 && Math.abs(dx) > Math.abs(dy) * 1.5) {
            t.isSeeking = true;
            t.seekDelta = Math.round(dx / 8); // 8px ≈ 1s
            showHint({ type: 'seek', delta: t.seekDelta }, 8000);
            resetToolbarTimer();
        }
    };

    const handleTouchEnd = (e: React.TouchEvent) => {
        const t = touch.current;
        const elapsed = Date.now() - t.startTime;
        if (t.longPressTimer) { clearTimeout(t.longPressTimer); t.longPressTimer = null; }
        if (t.isLongPress) {
            if (artRef.current) {
                try { artRef.current.video.playbackRate = 1.0; } catch { }
            }
            setHint(null);
            return;
        }
        if (t.isSeeking) {
            if (artRef.current) {
                const duration = Number.isFinite(artRef.current.duration) ? artRef.current.duration : Infinity;
                const target = Math.max(0, artRef.current.currentTime + t.seekDelta);
                artRef.current.seek = Math.min(target, duration);
            }
            setHint(null);
            return;
        }
        if (elapsed < 350) {
            const now = Date.now();
            if (now - t.lastTapTime < 350) {
                if (t.singleTapTimer) { clearTimeout(t.singleTapTimer); t.singleTapTimer = null; }
                t.lastTapTime = 0;
                if (artRef.current) {
                    if (artRef.current.playing) { artRef.current.pause(); showHint({ type: 'pause' }); }
                    else { artRef.current.play(); showHint({ type: 'play' }); }
                }
            } else {
                t.lastTapTime = now;
                if (t.singleTapTimer) clearTimeout(t.singleTapTimer);
                t.singleTapTimer = setTimeout(() => {
                    toggleToolbar();
                    t.singleTapTimer = null;
                }, 260);
            }
        }
    };

    const persistProgress = useCallback(() => {
        const art = artRef.current;
        if (!art) return;
        if (!Number.isFinite(art.currentTime) || art.currentTime <= 0) return;
        const rawDuration = Number.isFinite(art.duration) && art.duration > 0 ? Math.floor(art.duration) : 0;
        const position = rawDuration > 0
            ? Math.min(Math.floor(art.currentTime), rawDuration)
            : Math.floor(art.currentTime);
        const record = savePlayRecord(file.rel_path, file.name, position, rawDuration);
        if (record) onRecordSaved?.(record);
    }, [file.name, file.rel_path, onRecordSaved]);

    const safeSeekTo = useCallback((rawTarget: number) => {
        const art = artRef.current;
        if (!art) return;
        const d = Number.isFinite(art.duration) && art.duration > 0 ? art.duration : 0;
        if (d <= 1) return;
        if (!Number.isFinite(rawTarget)) return;
        const cap = Math.max(0, d - 0.25);
        const target = Math.max(0, Math.min(cap, rawTarget));
        const wasPlaying = !!art.playing;
        try {
            // 直接操作 video.currentTime，对 flv/ts/mp4 都适用
            // 注：mpegts 播放器对象上并没有 .currentTime setter，只能用 video 元素本身
            if (art.video) {
                art.video.currentTime = target;
            } else {
                art.seek = target;
            }
        } catch {
            try { art.seek = target; } catch { return; }
        }
        setCurrentTime(target);
        if (wasPlaying) {
            window.setTimeout(() => {
                try {
                    if (artRef.current && !artRef.current.playing) artRef.current.play();
                } catch { }
            }, 50);
        }
    }, []);

    const syncPlayerState = useCallback(() => {
        const art = artRef.current;
        if (!art) return;
        const d = Number.isFinite(art.duration) && art.duration > 0 ? art.duration : 0;
        const c = Number.isFinite(art.currentTime) && art.currentTime > 0 ? art.currentTime : 0;
        setDuration(d);
        if (!seekingBySliderRef.current) setCurrentTime(Math.min(c, d || c));
        setPlaying(!!art.playing);
        setIsFullscreen(!!(art.fullscreen || art.fullscreenWeb));
    }, []);

    const seekBy = (delta: number) => {
        const art = artRef.current;
        if (!art) return;
        safeSeekTo(art.currentTime + delta);
        syncPlayerState();
    };

    const toggleFullscreen = () => {
        const art = artRef.current;
        if (!art) return;
        const video = art.video as HTMLVideoElement & {
            webkitEnterFullscreen?: () => void;
            webkitExitFullscreen?: () => void;
        };
        // iOS Safari 不支持标准 Fullscreen API，用原生 webkit 全屏
        if (video?.webkitEnterFullscreen && !document.fullscreenEnabled) {
            if (isFullscreen) {
                try { video.webkitExitFullscreen?.(); } catch { }
            } else {
                try { video.webkitEnterFullscreen(); } catch { }
            }
            resetToolbarTimer();
            return;
        }
        try {
            if (art.fullscreen || art.fullscreenWeb) {
                art.fullscreen = false;
                art.fullscreenWeb = false;
            } else {
                art.fullscreen = true;
                if (!art.fullscreen) art.fullscreenWeb = true;
            }
        } catch {
            try { art.fullscreenWeb = !art.fullscreenWeb; } catch { }
        }
        syncPlayerState();
        resetToolbarTimer();
    };

    const togglePlay = () => {
        const art = artRef.current;
        if (!art) return;
        if (art.playing) art.pause();
        else art.play();
        syncPlayerState();
        resetToolbarTimer();
    };

    useEffect(() => {
        if (!containerRef.current) return;
        const ext = (file.rel_path.split('.').pop() || '').toLowerCase();
        streamTypeRef.current = ext === 'flv' ? 'flv' : ext === 'ts' ? 'ts' : 'other';
        mpegtsPlayerRef.current = null;

        const makeCustomType = (type: 'flv' | 'mpegts') => (video: HTMLVideoElement, url: string, art: Artplayer) => {
            if (!mpegtsjs.isSupported()) { art.notice.show = `不支持播放格式: ${type}`; return; }
            const player = mpegtsjs.createPlayer({ type: type === 'flv' ? 'flv' : 'mpegts', url, hasVideo: true, hasAudio: true });
            player.attachMediaElement(video);
            player.load();
            mpegtsPlayerRef.current = player;
            art.on('destroy', () => {
                try { player.unload(); } catch { }
                try { player.detachMediaElement(); } catch { }
                try { player.destroy(); } catch { }
                if (mpegtsPlayerRef.current === player) mpegtsPlayerRef.current = null;
            });
        };

        const art = new Artplayer({
            container: containerRef.current,
            url: playUrl,
            title: file.name,
            volume: 0.9,
            autoplay: true,
            autoSize: true,
            pip: false,
            setting: false,
            playbackRate: false,
            aspectRatio: false,
            fullscreen: true,
            fullscreenWeb: true,
            miniProgressBar: false,
            mutex: true,
            hotkey: false,
            lang: 'zh-cn',
            theme: '#4fc3f7',
            moreVideoAttr: {
                playsInline: true,
                webkitPlaysinline: true,
                x5Playsinline: true,
            } as any,
            customType: {
                flv: makeCustomType('flv'),
                ts: makeCustomType('mpegts'),
            },
        });

        artRef.current = art;

        art.on('ready', () => {
            art.video.controls = false;
            art.video.setAttribute('playsinline', 'true');
            art.video.setAttribute('webkit-playsinline', 'true');
            art.video.setAttribute('x5-playsinline', 'true');
            // @ts-ignore
            art.video.disablePictureInPicture = true;
            // iOS 原生全屏事件（webkitEnterFullscreen）
            const vid = art.video as HTMLVideoElement & { webkitEnterFullscreen?: () => void };
            if (vid.webkitEnterFullscreen) {
                art.video.addEventListener('webkitbeginfullscreen', () => setIsFullscreen(true));
                art.video.addEventListener('webkitendfullscreen', () => { setIsFullscreen(false); syncPlayerState(); });
            }
            if (resumeTime > 5) {
                art.video.currentTime = resumeTime;
                art.notice.show = `从 ${formatTime(resumeTime)} 继续播放`;
            }
            syncPlayerState();
        });
        art.on('video:play', syncPlayerState);
        art.on('video:pause', syncPlayerState);
        art.on('video:timeupdate', syncPlayerState);
        art.on('video:loadedmetadata', syncPlayerState);

        // 每 10 秒保存一次进度
        saveTimerRef.current = setInterval(() => {
            persistProgress();
        }, 10000);
        uiTimerRef.current = setInterval(syncPlayerState, 300);

        art.on('destroy', persistProgress);

        return () => {
            if (saveTimerRef.current) clearInterval(saveTimerRef.current);
            if (uiTimerRef.current) clearInterval(uiTimerRef.current);
            persistProgress();
            if (artRef.current) {
                try { artRef.current.video.pause(); } catch { }
                try { artRef.current.destroy(true); } catch { }
                artRef.current = null;
            }
            mpegtsPlayerRef.current = null;
        };
    }, [file.name, file.rel_path, persistProgress, playUrl, resumeTime, syncPlayerState]);

    useEffect(() => {
        const handleVisibility = () => {
            if (document.visibilityState === 'hidden') persistProgress();
        };
        const handlePageLeave = () => persistProgress();
        document.addEventListener('visibilitychange', handleVisibility);
        window.addEventListener('pagehide', handlePageLeave);
        window.addEventListener('beforeunload', handlePageLeave);
        return () => {
            document.removeEventListener('visibilitychange', handleVisibility);
            window.removeEventListener('pagehide', handlePageLeave);
            window.removeEventListener('beforeunload', handlePageLeave);
        };
    }, [persistProgress]);

    const onSliderInput = (value: number) => {
        setSeekingBySlider(true);
        seekingBySliderRef.current = true;
        setSliderValue(value);
        resetToolbarTimer();
    };

    const commitSliderSeek = () => {
        if (!seekingBySliderRef.current) return;
        safeSeekTo(sliderValue);
        syncPlayerState();
        setSeekingBySlider(false);
        seekingBySliderRef.current = false;
    };

    const shownTime = seekingBySlider ? sliderValue : currentTime;

    useEffect(() => {
        const root = pageRef.current;
        if (!root) return;
        const prevent = (e: Event) => e.preventDefault();
        // 防止页面滚动/弹射（INPUT 范围除外，保留进度条滚动）
        const preventScroll = (e: TouchEvent) => {
            const target = e.target as HTMLElement | null;
            if (target && (target.tagName === 'INPUT' || target.closest('input[type="range"]'))) return;
            if (e.cancelable) e.preventDefault();
        };
        root.addEventListener('contextmenu', prevent);
        root.addEventListener('selectstart', prevent);
        root.addEventListener('touchmove', preventScroll, { passive: false });
        const bodyStyle = document.body.style as CSSStyleDeclaration & { webkitTouchCallout?: string; webkitUserSelect?: string };
        const oldCallout = bodyStyle.webkitTouchCallout || '';
        const oldUserSelect = document.body.style.userSelect;
        const oldWebkitUserSelect = bodyStyle.webkitUserSelect || '';
        bodyStyle.webkitTouchCallout = 'none';
        document.body.style.userSelect = 'none';
        bodyStyle.webkitUserSelect = 'none';
        return () => {
            root.removeEventListener('contextmenu', prevent);
            root.removeEventListener('selectstart', prevent);
            root.removeEventListener('touchmove', preventScroll);
            bodyStyle.webkitTouchCallout = oldCallout;
            document.body.style.userSelect = oldUserSelect;
            bodyStyle.webkitUserSelect = oldWebkitUserSelect;
        };
    }, []);

    useEffect(() => {
        const overlay = overlayRef.current;
        if (!overlay) return;
        const preventTouch = (e: TouchEvent) => {
            // 允许在进度条上滑动
            if (e.target && (e.target as HTMLElement).tagName === 'INPUT') return;
            if (e.cancelable) e.preventDefault();
        };
        overlay.addEventListener('touchstart', preventTouch, { passive: false });
        overlay.addEventListener('touchmove', preventTouch, { passive: false });
        return () => {
            overlay.removeEventListener('touchstart', preventTouch);
            overlay.removeEventListener('touchmove', preventTouch);
        };
    }, []);

    const renderHintIcon = () => {
        if (!hint) return null;
        switch (hint.type) {
            case 'play': return <><IconPlay /><span>播放</span></>;
            case 'pause': return <><IconPause /><span>暂停</span></>;
            case 'seek': return <>
                {hint.delta >= 0 ? <IconSeekForward /> : <IconSeekBack />}
                <span>{hint.delta >= 0 ? '+' : ''}{hint.delta}s</span>
            </>;
            case 'speed': return <><IconSpeed /><span>2× 快进</span></>;
            default: return null;
        }
    };

    return (
        <div className="player-page" ref={pageRef}>
            <button className="player-floating-back-btn" onClick={onBack} aria-label="返回">
                <IconBack />
            </button>

            {/* 顶部工具栏 */}
            <div className={`player-toolbar ${toolbarVisible ? 'toolbar-visible' : 'toolbar-hidden'}`}>
                <button className="player-back-btn" onClick={onBack}>
                    <IconBack />
                </button>
                <span className="player-title">{file.name}</span>
            </div>

            {/* Artplayer 容器 */}
            <div ref={containerRef} className="art-container" />

            {/* 手势蒙层（透明，接管触摸事件） */}
            <div
                ref={overlayRef}
                className="gesture-overlay"
                onTouchStart={handleTouchStart}
                onTouchMove={handleTouchMove}
                onTouchEnd={handleTouchEnd}
                onTouchCancel={handleTouchEnd}
                onContextMenu={(e) => e.preventDefault()}
                // onClick 作为备用（部分 iOS 场景 touchend 可能被系统吞掉）
                onClick={() => { toggleToolbar(); resetToolbarTimer(); }}
            />

            {/* 手势提示 */}
            {hint && (
                <div className="gesture-hint">
                    {renderHintIcon()}
                </div>
            )}

            <div
                className={`player-custom-controls ${toolbarVisible ? 'controls-visible' : 'controls-hidden'}`}
                onTouchStart={e => e.stopPropagation()}
                onClick={e => e.stopPropagation()}
            >
                <button className="player-control-btn" onClick={() => seekBy(-10)} aria-label="后退10秒">
                    <IconBackward10 />
                </button>
                <button className="player-control-btn player-control-main" onClick={togglePlay} aria-label={playing ? '暂停' : '播放'}>
                    {playing ? <IconControlPause /> : <IconControlPlay />}
                </button>
                <button className="player-control-btn" onClick={() => seekBy(10)} aria-label="前进10秒">
                    <IconForward10 />
                </button>
                <button className="player-control-btn" onClick={toggleFullscreen} aria-label={isFullscreen ? '退出全屏' : '全屏'}>
                    {isFullscreen ? <IconFullscreenExit /> : <IconFullscreen />}
                </button>
                <span className="player-time-label">{formatTime(Math.floor(shownTime))}</span>
                <input
                    type="range"
                    className="player-progress"
                    min={0}
                    max={Math.max(duration, 1)}
                    step={0.1}
                    value={Math.min(Math.max(shownTime, 0), Math.max(duration, 1))}
                    onChange={(e) => onSliderInput(Number(e.target.value))}
                    onMouseUp={commitSliderSeek}
                    onTouchEnd={commitSliderSeek}
                    onTouchCancel={commitSliderSeek}
                    onPointerUp={commitSliderSeek}
                    onPointerCancel={commitSliderSeek}
                />
                <span className="player-time-label">{formatTime(Math.floor(duration || 0))}</span>
            </div>
        </div>
    );
};

function formatTime(s: number): string {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, '0')}`;
}

// ===== 单个主播视频网格 =====
interface VideoGridProps {
    room: VideoRoomInfo;
    onBack: () => void;
    onPlay: (file: VideoFileInfo) => void;
}
const VideoGrid: React.FC<VideoGridProps> = ({ room, onBack, onPlay }) => {
    const [files, setFiles] = useState<VideoFileInfo[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        api.getVideoFiles(room.folder_path).then((d: any) => { setFiles(d || []); setLoading(false); }).catch(() => setLoading(false));
    }, [room.folder_path]);

    const getThumbnailUrl = (relPath: string) => `/api/thumbnail/${relPath.split('/').map(encodeURIComponent).join('/')}`;

    return (
        <div>
            <div className="video-grid-header">
                <Button icon={<ArrowLeftOutlined />} onClick={onBack}>返回</Button>
                <span className="grid-host-name">{room.host_name}</span>
                <Tag color="blue">{room.platform}</Tag>
                <Text type="secondary" style={{ fontSize: 12 }}>{files.length} 个视频</Text>
            </div>
            {loading ? (
                <div style={{ textAlign: 'center', padding: 60 }}><Spin size="large" /></div>
            ) : files.length === 0 ? (
                <Empty description="暂无视频" />
            ) : (
                <Row gutter={[12, 12]}>
                    {files.map(f => (
                        <Col key={f.rel_path} xs={24} sm={12} md={8} lg={6}>
                            <Card
                                className="video-file-card" hoverable
                                onClick={() => onPlay(f)}
                                cover={
                                    <div className="thumbnail-container">
                                        <img alt="缩略图" src={getThumbnailUrl(f.rel_path)} className="thumbnail-img"
                                            onError={e => {
                                                (e.target as HTMLImageElement).style.display = 'none';
                                                const p = (e.target as HTMLImageElement).nextElementSibling as HTMLElement;
                                                if (p) p.style.display = 'flex';
                                            }}
                                        />
                                        <div className="thumbnail-placeholder" style={{ display: 'none' }}>
                                            <VideoCameraOutlined style={{ fontSize: 36, color: '#bbb' }} />
                                        </div>
                                        <div className="play-overlay">▶</div>
                                    </div>
                                }
                            >
                                <div className="file-card-body">
                                    <Tooltip title={f.name}>
                                        <div className="file-name">{f.name}</div>
                                    </Tooltip>
                                    <div className="file-meta">
                                        <span>{Utils.byteSizeToHumanReadableFileSize(f.size)}</span>
                                        <span style={{ fontSize: 10 }}>{Utils.timestampToHumanReadable(f.mod_time)}</span>
                                    </div>
                                </div>
                            </Card>
                        </Col>
                    ))}
                </Row>
            )}
        </div>
    );
};

// ===== 主页：视频库 =====
const VideoLibrary: React.FC = () => {
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const [rooms, setRooms] = useState<VideoRoomInfo[]>([]);
    const [loading, setLoading] = useState(true);
    const [showAdd, setShowAdd] = useState(false);
    const [lastRecord, setLastRecord] = useState<PlayRecord | null>(null);
    const selectedRoomPath = searchParams.get('room') || '';
    const playingRelPath = searchParams.get('play') || '';
    const playingName = searchParams.get('name') || '';

    const loadRooms = useCallback(() => {
        setLoading(true);
        api.getVideoLibrary().then((d: any) => { setRooms(d || []); setLoading(false); }).catch(() => setLoading(false));
    }, []);

    useEffect(() => {
        loadRooms();
        const rec = loadPlayRecord();
        if (rec) {
            // 校验历史内的文件是否仍存在，如已删除则清除记录
            const fileUrl = `/files/${rec.relPath.split('/').map(encodeURIComponent).join('/')}`;
            fetch(fileUrl, { method: 'HEAD' })
                .then(r => {
                    if (r.ok) {
                        setLastRecord(rec);
                    } else {
                        // 文件不存在（404），清除历史记录
                        localStorage.removeItem(HISTORY_KEY);
                        setLastRecord(null);
                    }
                })
                .catch(() => {
                    // 网络错误时保留记录，避免误删
                    setLastRecord(rec);
                });
        }
    }, [loadRooms]);

    const getThumbnailUrl = (latestVideo: string) => latestVideo ? `/api/thumbnail/${latestVideo.split('/').map(encodeURIComponent).join('/')}` : '';

    const selectedRoom = useMemo(
        () => rooms.find(r => r.folder_path === selectedRoomPath) || null,
        [rooms, selectedRoomPath],
    );

    const playingFile = useMemo<VideoFileInfo | null>(() => {
        if (!playingRelPath) return null;
        const inferredName = playingName || playingRelPath.split('/').pop() || '视频';
        return {
            name: inferredName,
            rel_path: playingRelPath,
            size: 0,
            mod_time: Math.floor(Date.now() / 1000),
        };
    }, [playingName, playingRelPath]);

    const updateSearch = useCallback((next: URLSearchParams, replace = false) => {
        const search = next.toString();
        navigate({ search: search ? `?${search}` : '' }, { replace });
    }, [navigate]);

    const openRoom = useCallback((room: VideoRoomInfo) => {
        const next = new URLSearchParams(searchParams);
        next.set('room', room.folder_path);
        next.delete('play');
        next.delete('name');
        updateSearch(next);
    }, [searchParams, updateSearch]);

    const closeRoom = useCallback(() => {
        const next = new URLSearchParams(searchParams);
        next.delete('room');
        next.delete('play');
        next.delete('name');
        updateSearch(next);
    }, [searchParams, updateSearch]);

    const openPlayer = useCallback((file: VideoFileInfo, room?: VideoRoomInfo) => {
        const next = new URLSearchParams(searchParams);
        if (room) next.set('room', room.folder_path);
        next.set('play', file.rel_path);
        next.set('name', file.name);
        updateSearch(next);
    }, [searchParams, updateSearch]);

    const closePlayer = useCallback(() => {
        const next = new URLSearchParams(searchParams);
        next.delete('play');
        next.delete('name');
        updateSearch(next);
    }, [searchParams, updateSearch]);

    if (playingFile) {
        return (
            <div className="video-library-container">
                <VideoPlayer file={playingFile} onBack={closePlayer} onRecordSaved={setLastRecord} />
            </div>
        );
    }

    if (selectedRoom) {
        return (
            <div className="video-library-container">
                <VideoGrid room={selectedRoom} onBack={closeRoom} onPlay={(f) => openPlayer(f, selectedRoom)} />
            </div>
        );
    }

    return (
        <div className="video-library-container">
            <div className="video-library-header">
                <div className="header-left">
                    <Title level={4} style={{ margin: 0 }}>
                        <VideoCameraOutlined style={{ marginRight: 8 }} />视频库
                    </Title>
                    {!loading && <Text type="secondary" style={{ fontSize: 13 }}>共 {rooms.length} 位主播</Text>}
                </div>
                <Button type="primary" icon={<PlusOutlined />} size="large" onClick={() => setShowAdd(true)}>
                    添加直播间
                </Button>
            </div>

            {/* 继续观看横幅 */}
            {lastRecord && lastRecord.duration > 10 && lastRecord.position > 5 && (
                <div className="continue-banner" onClick={() => {
                    const file: VideoFileInfo = { name: lastRecord.name, rel_path: lastRecord.relPath, size: 0, mod_time: lastRecord.savedAt / 1000 };
                    openPlayer(file);
                }}>
                    <PlayCircleOutlined className="continue-icon" />
                    <div className="continue-info">
                        <div className="continue-title">继续观看</div>
                        <div className="continue-name">{lastRecord.name}</div>
                        <div className="continue-progress-bar">
                            <div className="continue-progress-fill" style={{ width: `${Math.min(100, (lastRecord.position / lastRecord.duration) * 100).toFixed(1)}%` }} />
                        </div>
                        <div className="continue-time">{formatTime(lastRecord.position)} / {formatTime(lastRecord.duration)}</div>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="video-library-loading"><Spin size="large" tip="正在扫描录播文件..." /></div>
            ) : rooms.length === 0 ? (
                <Empty
                    image={<FolderOpenOutlined style={{ fontSize: 60, color: '#bbb' }} />}
                    description={
                        <Space direction="vertical" align="center">
                            <span>暂无录制视频</span>
                            <Button type="primary" icon={<PlusOutlined />} onClick={() => setShowAdd(true)}>添加直播间</Button>
                        </Space>
                    }
                />
            ) : (
                <Row gutter={[16, 16]}>
                    {rooms.map(room => (
                        <Col key={room.folder_path} xs={24} sm={12} lg={8} xl={6}>
                            <Card
                                className="video-room-card" hoverable
                                onClick={() => openRoom(room)}
                                cover={
                                    room.latest_video ? (
                                        <div className="thumbnail-container">
                                            <img alt="缩略图" src={getThumbnailUrl(room.latest_video)} className="thumbnail-img"
                                                onError={e => {
                                                    (e.target as HTMLImageElement).style.display = 'none';
                                                    const p = (e.target as HTMLImageElement).nextElementSibling as HTMLElement;
                                                    if (p) p.style.display = 'flex';
                                                }}
                                            />
                                            <div className="thumbnail-placeholder" style={{ display: 'none' }}>
                                                <VideoCameraOutlined style={{ fontSize: 40, color: '#bbb' }} />
                                            </div>
                                        </div>
                                    ) : (
                                        <div className="thumbnail-placeholder">
                                            <VideoCameraOutlined style={{ fontSize: 40, color: '#bbb' }} />
                                        </div>
                                    )
                                }
                            >
                                <Card.Meta
                                    title={
                                        <Tooltip title={room.host_name}>
                                            <span className="host-name">{room.host_name}</span>
                                        </Tooltip>
                                    }
                                    description={
                                        <div className="card-meta">
                                            <div className="card-meta-row">
                                                <Tag color="blue">{room.platform}</Tag>
                                                <Tag color="green">{room.video_count} 个视频</Tag>
                                            </div>
                                            <div className="card-meta-row" style={{ color: '#999', fontSize: 12 }}>
                                                <ClockCircleOutlined style={{ marginRight: 4 }} />
                                                {Utils.timestampToHumanReadable(room.latest_video_at)}
                                            </div>
                                            <div style={{ color: '#bbb', fontSize: 11 }}>
                                                {Utils.byteSizeToHumanReadableFileSize(room.total_size)}
                                            </div>
                                        </div>
                                    }
                                />
                            </Card>
                        </Col>
                    ))}
                </Row>
            )}

            <AddRoomModal visible={showAdd} onClose={() => setShowAdd(false)} onAdded={loadRooms} />
        </div>
    );
};

export default VideoLibrary;
