import { expect, expectTypeOf, it } from 'vitest'

it('registers DOM matchers with synchronous and asynchronous return types', async () => {
  const button = document.createElement('button')
  button.textContent = 'Continue'
  document.body.append(button)

  try {
    expectTypeOf(expect(button).toBeVisible()).toEqualTypeOf<void>()
    expect(button).toHaveTextContent('Continue')

    const assertion = expect(Promise.resolve(button)).resolves.toBeVisible()
    expectTypeOf(assertion).toEqualTypeOf<Promise<void>>()
    await assertion
  } finally {
    button.remove()
  }
})
