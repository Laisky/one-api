import { useTranslation } from 'react-i18next';

export function ChatPage() {
  const chatLink = localStorage.getItem('chat_link');
  const { t } = useTranslation();

  if (!chatLink) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <h2 className="text-2xl font-semibold text-muted-foreground mb-2">{t('chat.not_available')}</h2>
          <p className="text-muted-foreground">{t('chat.not_configured')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full">
      <iframe src={chatLink} className="w-full h-full border-0" title={t('chat.iframe_title')} />
    </div>
  );
}

export default ChatPage;
