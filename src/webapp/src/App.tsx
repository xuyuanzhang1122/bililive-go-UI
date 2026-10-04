import React, { useEffect, useState } from 'react';
import { Alert, Spin } from 'antd';
import { initializeWebAuth } from './utils/common';
import './App.css';
import { Routes, Route } from 'react-router-dom';
import RootLayout from './component/layout/index';
import LiveInfo from './component/live-info/index';
import LiveList from './component/live-list/index';

import ConfigInfo from './component/config-info/index';
import FileList from './component/file-list/index';
import TaskPage from './component/task-page/index';
import IOStats from './component/io-stats/index';
import VideoLibrary from './component/video-library/index';
import HistoryPage from './component/history-page/index';

const App: React.FC = () => {
  const [authReady, setAuthReady] = useState(false);
  const [authError, setAuthError] = useState('');
  useEffect(() => {
    let active = true;
    initializeWebAuth().then(() => { if (active) setAuthReady(true); })
      .catch(error => { if (active) setAuthError(error instanceof Error ? error.message : '认证初始化失败'); });
    return () => { active = false; };
  }, []);
  if (authError) return <Alert type="error" message={authError} description="请检查服务端连接后刷新页面。" />;
  if (!authReady) return <Spin tip="正在初始化认证"><div style={{ minHeight: 160 }} /></Spin>;

  return (
    <>
      <RootLayout>
        <Routes>
          <Route path="/iostats/*" element={<IOStats />} />
          <Route path="/tasks/*" element={<TaskPage />} />
          <Route path="/fileList/*" element={<FileList />} />
          <Route path="/configInfo/*" element={<ConfigInfo />} />
          <Route path="/videoLibrary" element={<VideoLibrary />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/liveInfo" element={<LiveInfo />} />
          <Route path="/liveList" element={<LiveList />} />
          <Route path="/" element={<VideoLibrary />} />
        </Routes>
      </RootLayout>
    </>
  );
}

export default App;

