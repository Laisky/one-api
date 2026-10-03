// duplicateActionTranslations keeps duplicate labels identical across resource types in every supported locale.
export const duplicateActionTranslations = {
  en: {
    action: 'Duplicate',
    pending: 'Duplicating...',
    channel_success: 'Created channel "{{name}}".',
    channel_created: 'Channel duplicated.',
    channel_failed: 'Unable to duplicate the channel.',
    channel_refresh_failed: 'The channel was created, but the list could not be refreshed. Refresh the page.',
  },
  zh: {
    action: '创建副本',
    pending: '正在创建副本...',
    channel_success: '已创建渠道“{{name}}”。',
    channel_created: '已创建渠道副本。',
    channel_failed: '无法创建渠道副本。',
    channel_refresh_failed: '渠道已创建，但列表刷新失败，请刷新页面。',
  },
  fr: {
    action: 'Dupliquer',
    pending: 'Duplication...',
    channel_success: 'Le canal « {{name}} » a été créé.',
    channel_created: 'Le canal a été dupliqué.',
    channel_failed: 'Impossible de dupliquer le canal.',
    channel_refresh_failed: 'Le canal a été créé, mais la liste n’a pas pu être actualisée. Actualisez la page.',
  },
  es: {
    action: 'Duplicar',
    pending: 'Duplicando...',
    channel_success: 'Se ha creado el canal «{{name}}».',
    channel_created: 'Se ha duplicado el canal.',
    channel_failed: 'No se puede duplicar el canal.',
    channel_refresh_failed: 'El canal se ha creado, pero no se ha podido actualizar la lista. Actualiza la página.',
  },
  ja: {
    action: '複製',
    pending: '複製中...',
    channel_success: 'チャネル「{{name}}」を作成しました。',
    channel_created: 'チャネルを複製しました。',
    channel_failed: 'チャネルを複製できません。',
    channel_refresh_failed: 'チャネルは作成されましたが、一覧を更新できませんでした。ページを再読み込みしてください。',
  },
} as const;
