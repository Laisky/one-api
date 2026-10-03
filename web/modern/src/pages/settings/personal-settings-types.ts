export type PersonalForm = {
  username: string;
  display_name?: string;
  email?: string;
};

export type PasskeyInfo = {
  id?: number;
  uuid?: string;
  credential_name: string;
  sign_count: number;
  created_at: number;
};

export type OAuthBindings = {
  github_id: string;
  wechat_id: string;
  lark_id: string;
  oidc_id: string;
};
