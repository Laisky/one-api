import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Button } from 'semantic-ui-react';
import { API, showError } from '../../helpers';
import { marked } from 'marked';
import { Link } from 'react-router-dom';

const About = () => {
  const { t } = useTranslation();
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
      setAbout(t('about.loading_failed'));
    }
    setAboutLoaded(true);
  };

  useEffect(() => {
    displayAbout().then();
  }, []);

  return (
    <>
      {aboutLoaded && about === '' ? (
        <div className='dashboard-container'>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header className='header'>{t('about.title')}</Card.Header>
              <p>{t('about.description')}</p>
              <p>
                <Button as={Link} to='/models' primary>
                  {t('about.view_models', 'View Supported Models')}
                </Button>
              </p>
              {t('about.repository')}
              <a href='https://github.com/Laisky/one-api'>
                https://github.com/Laisky/one-api
              </a>
            </Card.Content>
          </Card>
        </div>
      ) : (
        <>
          {about.startsWith('https://') ? (
            <iframe
              src={about}
              style={{ width: '100%', height: '100vh', border: 'none' }}
            />
          ) : (
            <div className='dashboard-container'>
              <Card fluid className='chart-card'>
                <Card.Content>
                  <div style={{ marginTop: '20px', textAlign: 'center' }}>
                    <Button as={Link} to='/models' primary>
                      {t('about.view_models', 'View Supported Models')}
                    </Button>
                  </div>
                  <div
                    style={{ fontSize: 'larger' }}
                    dangerouslySetInnerHTML={{ __html: about }}
                  ></div>
                </Card.Content>
              </Card>
            </div>
          )}
        </>
      )}
    </>
  );
};

export default About;
