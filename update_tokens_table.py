import sys

with open('web/default/src/components/TokensTable.js', 'r') as f:
    content = f.read()

# 1. Visibility toggle
old_v = """                  <Popup
                    trigger={
                      <Button
                        size="mini"
                        icon
                        onClick={() => toggleKeyVisibility(token.id)}
                      >
                        <Icon name={showKeys[token.id] ? 'eye slash' : 'eye'} />
                      </Button>
                    }
                    content={showKeys[token.id] ? t('common:hide') : t('common:show')}
                    basic
                    inverted
                  />"""
new_v = """                  <Popup
                    trigger={
                      <Button
                        size="mini"
                        icon
                        onClick={() => toggleKeyVisibility(token.id)}
                        aria-label={showKeys[token.id] ? t('common:hide', 'Hide') : t('common:show', 'Show')}
                      >
                        <Icon name={showKeys[token.id] ? 'eye slash' : 'eye'} />
                      </Button>
                    }
                    content={showKeys[token.id] ? t('common:hide', 'Hide') : t('common:show', 'Show')}
                    basic
                    inverted
                  />"""
content = content.replace(old_v, new_v)

# 2. Copy button
old_c = """                  <Popup
                    trigger={
                      <Button
                        size="mini"
                        icon
                        onClick={() => copyTokenKey(token.key)}
                      >
                        <Icon name="copy" />
                      </Button>
                    }
                    content={t('common:copy')}
                    basic
                    inverted
                  />"""
new_c = """                  <Popup
                    trigger={
                      <Button
                        size="mini"
                        icon
                        onClick={() => copyTokenKey(token.key)}
                        aria-label={t('common:copy', 'Copy')}
                      >
                        <Icon name="copy" />
                      </Button>
                    }
                    content={t('common:copy', 'Copy')}
                    basic
                    inverted
                  />"""
content = content.replace(old_c, new_c)

# 3. Enable/Disable
old_e = """                  <Popup
                    trigger={
                      <Button
                        size='small'
                        positive={token.status === 1}
                        negative={token.status !== 1}
                        onClick={() => {
                          manageToken(
                            token.id,
                            token.status === 1 ? 'disable' : 'enable',
                            idx
                          );
                        }}
                      >
                        {token.status === 1 ? (
                          <Icon name='pause' />
                        ) : (
                          <Icon name='play' />
                        )}
                      </Button>
                    }
                    content={
                      token.status === 1
                        ? t('common:disable')
                        : t('common:enable')
                    }
                    basic
                    inverted
                  />"""
new_e = """                  <Popup
                    trigger={
                      <Button
                        size='small'
                        positive={token.status === 1}
                        negative={token.status !== 1}
                        onClick={() => {
                          manageToken(
                            token.id,
                            token.status === 1 ? 'disable' : 'enable',
                            idx
                          );
                        }}
                        aria-label={
                          token.status === 1
                            ? t('common:disable', 'Disable')
                            : t('common:enable', 'Enable')
                        }
                      >
                        {token.status === 1 ? (
                          <Icon name='pause' />
                        ) : (
                          <Icon name='play' />
                        )}
                      </Button>
                    }
                    content={
                      token.status === 1
                        ? t('common:disable', 'Disable')
                        : t('common:enable', 'Enable')
                    }
                    basic
                    inverted
                  />"""
content = content.replace(old_e, new_e)

# 4. Edit
old_ed = """                  <Popup
                    trigger={
                      <Button
                        size='small'
                        color='blue'
                        as={Link}
                        to={'/token/edit/' + token.id}
                      >
                        <Icon name='edit' />
                      </Button>
                    }
                    content={t('common:edit')}
                    basic
                    inverted
                  />"""
new_ed = """                  <Popup
                    trigger={
                      <Button
                        size='small'
                        color='blue'
                        as={Link}
                        to={'/token/edit/' + token.id}
                        aria-label={t('common:edit', 'Edit')}
                      >
                        <Icon name='edit' />
                      </Button>
                    }
                    content={t('common:edit', 'Edit')}
                    basic
                    inverted
                  />"""
content = content.replace(old_ed, new_ed)

# 5. Delete
old_d = """                  <Popup
                    trigger={
                      <Button
                        size='small'
                        negative
                        onClick={() => {
                          manageToken(token.id, 'delete', idx);
                        }}
                      >
                        <Icon name='trash' />
                      </Button>
                    }
                    content={t('common:delete')}
                    basic
                    inverted
                  />"""
new_d = """                  <Popup
                    trigger={
                      <Button
                        size='small'
                        negative
                        aria-label={t('common:delete', 'Delete')}
                      >
                        <Icon name='trash' />
                      </Button>
                    }
                    on='click'
                    flowing
                    hoverable
                  >
                    <Button
                      negative
                      onClick={() => {
                        manageToken(token.id, 'delete', idx);
                      }}
                    >
                      {t('token.confirm_delete', 'Delete Token')}
                    </Button>
                  </Popup>"""
content = content.replace(old_d, new_d)

with open('web/default/src/components/TokensTable.js', 'w') as f:
    f.write(content)
