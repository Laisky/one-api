import { showError as reportUIError } from '../helpers/utils';
import React, { useContext, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { UserContext } from '../context/User';

import { API, getLogo, getSystemName, showSuccess } from '../helpers';
import '../index.css';

import fireworks from 'react-fireworks';

import { IconHelpCircle, IconKey, IconUser } from '@douyinfe/semi-icons';
import { Avatar, Dropdown, Layout, Nav, Switch } from '@douyinfe/semi-ui';
import { stringToColor } from '../helpers/render';

// HeaderBar Buttons
let headerButtons = [
  {
    text: '关于',
    itemKey: 'about',
    to: '/about',
    icon: <IconHelpCircle />
  }
];

if (localStorage.getItem('chat_link')) {
  headerButtons.splice(1, 0, {
    name: '聊天',
    to: '/chat',
    icon: 'comments'
  });
}

const HeaderBar = () => {
  const [userState, userDispatch] = useContext(UserContext);
  let navigate = useNavigate();

  const [showSidebar, setShowSidebar] = useState(false);
  const [dark, setDark] = useState(false);
  const systemName = getSystemName();
  const logo = getLogo();
  var themeMode = localStorage.getItem('theme-mode') || 'system';
  const currentDate = new Date();
  // enable fireworks on new year(1.1 and 2.9-2.24)
  const isNewYear = (currentDate.getMonth() === 0 && currentDate.getDate() === 1) || (currentDate.getMonth() === 1 && currentDate.getDate() >= 9 && currentDate.getDate() <= 24);

  async function logout() {
    setShowSidebar(false);
    await API.get('/api/user/logout');
    showSuccess('注销成功!');
    userDispatch({ type: 'logout' });
    localStorage.removeItem('user');
    navigate('/login');
  }

  const handleNewYearClick = () => {
    fireworks.init('root', {});
    fireworks.start();
    setTimeout(() => {
      fireworks.stop();
      setTimeout(() => {
        window.location.reload();
      }, 10000);
    }, 3000);
  };

  // Function to get system preference
  const getSystemPreference = () => {
    if (typeof window !== 'undefined' && window.matchMedia) {
      return window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
    return false;
  };

  useEffect(() => {
    let shouldUseDark = false;

    if (themeMode === 'dark') {
      shouldUseDark = true;
    } else if (themeMode === 'system') {
      shouldUseDark = getSystemPreference();
    }

    switchMode(shouldUseDark);

    if (isNewYear) {
      console.log('Happy New Year!');
    }

    // Listen for system theme changes when in system mode
    if (themeMode === 'system' && typeof window !== 'undefined' && window.matchMedia) {
      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');

      const handleChange = (e) => {
        if (localStorage.getItem('theme-mode') === 'system') {
          switchMode(e.matches);
        }
      };

      mediaQuery.addEventListener('change', handleChange);
      return () => mediaQuery.removeEventListener('change', handleChange);
    }
  }, []);

  const switchMode = (model) => {
    const body = document.body;
    if (!model) {
      body.removeAttribute('theme-mode');
    } else {
      body.setAttribute('theme-mode', 'dark');
    }
    setDark(model);
  };

  const toggleTheme = () => {
    const currentTheme = localStorage.getItem('theme-mode') || 'system';
    let newTheme;

    if (currentTheme === 'light') {
      newTheme = 'dark';
    } else if (currentTheme === 'dark') {
      newTheme = 'system';
    } else {
      newTheme = 'light';
    }

    localStorage.setItem('theme-mode', newTheme);

    let shouldUseDark = false;
    if (newTheme === 'dark') {
      shouldUseDark = true;
    } else if (newTheme === 'system') {
      shouldUseDark = getSystemPreference();
    }

    switchMode(shouldUseDark);
  };
  return (
    <>
      <Layout>
        <div style={{ width: '100%' }}>
          <Nav
            mode={'horizontal'}
            // bodyStyle={{ height: 100 }}
            renderWrapper={({ itemElement, isSubNav, isInSubNav, props }) => {
              const routerMap = {
                about: '/about',
                login: '/login',
                register: '/register'
              };
              return (
                <Link
                  style={{ textDecoration: 'none' }}
                  to={routerMap[props.itemKey]}
                >
                  {itemElement}
                </Link>
              );
            }}
            selectedKeys={[]}
            // items={headerButtons}
            onSelect={key => {

            }}
            footer={
              <>
                {isNewYear &&
                  // happy new year
                  <Dropdown
                    position="bottomRight"
                    render={
                      <Dropdown.Menu>
                        <Dropdown.Item onClick={handleNewYearClick}>Happy New Year!!!</Dropdown.Item>
                      </Dropdown.Menu>
                    }
                  >
                    <Nav.Item itemKey={'new-year'} text={'🏮'} />
                  </Dropdown>
                }
                <Nav.Item itemKey={'about'} icon={<IconHelpCircle />} />
                <Switch checkedText="🌞" size={'large'} checked={dark} uncheckedText="🌙" onChange={toggleTheme} />
                {userState.user ?
                  <>
                    <Dropdown
                      position="bottomRight"
                      render={
                        <Dropdown.Menu>
                          <Dropdown.Item onClick={(...uiArgs) => logout(...uiArgs).catch(reportUIError)}>退出</Dropdown.Item>
                        </Dropdown.Menu>
                      }
                    >
                      <Avatar size="small" color={stringToColor(userState.user.username)} style={{ margin: 4 }}>
                        {userState.user.username[0]}
                      </Avatar>
                      <span>{userState.user.username}</span>
                    </Dropdown>
                  </>
                  :
                  <>
                    <Nav.Item itemKey={'login'} text={'登录'} icon={<IconKey />} />
                    <Nav.Item itemKey={'register'} text={'注册'} icon={<IconUser />} />
                  </>
                }
              </>
            }
          >
          </Nav>
        </div>
      </Layout>
    </>
  );
};

export default HeaderBar;
