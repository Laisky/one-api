/** MobileTableLanguage lists the application languages supported by the mobile list controls. */
export type MobileTableLanguage = 'en' | 'zh' | 'fr' | 'es' | 'ja';

const en = {
  actions: 'Actions',
  details: 'Details',
  less: 'Less',
  open_record: 'Open record',
  sort_by: 'Sort by',
  default_order: 'Default order',
  sort_ascending: 'Sort ascending',
  sort_descending: 'Sort descending',
  pagination: 'Pagination',
  page_status: 'Page {{page}} of {{pages}}',
};

/** mobileTableTranslations keeps every mobile control key aligned across the five application locales. */
export const mobileTableTranslations: Record<MobileTableLanguage, Record<keyof typeof en, string>> = {
  en,
  zh: {
    actions: '操作',
    details: '详情',
    less: '收起',
    open_record: '打开记录',
    sort_by: '排序字段',
    default_order: '默认顺序',
    sort_ascending: '升序排列',
    sort_descending: '降序排列',
    pagination: '分页',
    page_status: '第 {{page}} 页，共 {{pages}} 页',
  },
  fr: {
    actions: 'Actions',
    details: 'Détails',
    less: 'Réduire',
    open_record: 'Ouvrir la fiche',
    sort_by: 'Trier par',
    default_order: 'Ordre par défaut',
    sort_ascending: 'Trier par ordre croissant',
    sort_descending: 'Trier par ordre décroissant',
    pagination: 'Pagination',
    page_status: 'Page {{page}} sur {{pages}}',
  },
  es: {
    actions: 'Acciones',
    details: 'Detalles',
    less: 'Mostrar menos',
    open_record: 'Abrir registro',
    sort_by: 'Ordenar por',
    default_order: 'Orden predeterminado',
    sort_ascending: 'Orden ascendente',
    sort_descending: 'Orden descendente',
    pagination: 'Paginación',
    page_status: 'Página {{page}} de {{pages}}',
  },
  ja: {
    actions: '操作',
    details: '詳細',
    less: '折りたたむ',
    open_record: 'レコードを開く',
    sort_by: '並べ替え',
    default_order: '既定の順序',
    sort_ascending: '昇順で並べ替え',
    sort_descending: '降順で並べ替え',
    pagination: 'ページ送り',
    page_status: '{{pages}} ページ中 {{page}} ページ',
  },
};
