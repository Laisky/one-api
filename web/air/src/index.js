import { browserReporterReady } from './browser-sentry';
import { createErrorBoundary } from '../../legacy/browser-sentry.mjs';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import HeaderBar from './components/HeaderBar';
import Footer from './components/Footer';
import 'semantic-ui-css/semantic.min.css';
import './index.css';
import './mobile-override.css';
import { UserProvider } from './context/User';
import { ToastContainer } from 'react-toastify';
import 'react-toastify/dist/ReactToastify.css';
import { StatusProvider } from './context/Status';
import { Layout } from '@douyinfe/semi-ui';
import SiderBar from './components/SiderBar';

// Runtime configuration is bounded and settles before React mounts; early module errors are not buffered.
browserReporterReady.then((browserReporter) => {
  const SentryBoundary = createErrorBoundary(React, browserReporter);

  // initialization
  initVChartSemiTheme({
    isWatchingThemeSwitch: true,
  });

  const root = ReactDOM.createRoot(document.getElementById('root'));
  const { Sider, Content, Header } = Layout;
  root.render(
    <SentryBoundary>
      <React.StrictMode>
        <StatusProvider>
          <UserProvider>
            <BrowserRouter>
              <Layout>
                <Sider>
                  <SiderBar />
                </Sider>
                <Layout>
                  <Header>
                    <HeaderBar />
                  </Header>
                  <Content
                    style={{
                      padding: '24px',
                    }}
                  >
                    <App />
                  </Content>
                  <Layout.Footer>
                    <Footer></Footer>
                  </Layout.Footer>
                </Layout>
                <ToastContainer />
              </Layout>
            </BrowserRouter>
          </UserProvider>
        </StatusProvider>
      </React.StrictMode>
    </SentryBoundary>,
  );
});
