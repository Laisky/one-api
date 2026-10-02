import { showError as reportUIError } from '../../helpers/utils';
import React, { useEffect, useState } from 'react';
import { Header, Segment, Button } from 'semantic-ui-react';
import { API, showError } from '../../helpers';
import { marked } from 'marked';
import { Link } from 'react-router-dom';

const About = () => {
  const [about, setAbout] = useState('');
  const [aboutLoaded, setAboutLoaded] = useState(false);

  const displayAbout = async () => {
    setAbout(localStorage.getItem('about') || '');
    const res = await API.get('/api/about');
    const { success, message, data } = res.data;
    if (success) {
      let aboutContent = data;
      if (!data.startsWith('https://')) {
        aboutContent = marked.parse(data);
      }
      setAbout(aboutContent);
      localStorage.setItem('about', aboutContent);
    } else {
      showError(message);
      setAbout('加载关于内容失败...');
    }
    setAboutLoaded(true);
  };

  useEffect(() => {
    displayAbout().then().catch(reportUIError);
  }, []);

  return (
    <>
      {
        aboutLoaded && about === '' ? <>
          <Segment>
            <Header as='h3'>关于</Header>
            <p>可在设置页面设置关于内容，支持 HTML & Markdown</p>
            <p>
              <Button as={Link} to='/models' primary>
                查看支持的模型
              </Button>
            </p>
            项目仓库地址：
            <a href='https://github.com/Laisky/one-api'>
              https://github.com/Laisky/one-api
            </a>
          </Segment>
        </> : <>
          {
            about.startsWith('https://') ? <iframe
              src={about}
              style={{ width: '100%', height: '100vh', border: 'none' }}
            /> : (
              <>
                <div style={{ fontSize: 'larger' }} dangerouslySetInnerHTML={{ __html: about }}></div>
                <div style={{ marginTop: '20px', textAlign: 'center' }}>
                  <Button as={Link} to='/models' primary>
                    查看支持的模型
                  </Button>
                </div>
              </>
            )
          }
        </>
      }
    </>
  );
};


export default About;
