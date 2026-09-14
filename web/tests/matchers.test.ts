import { expect, expectTypeOf, it } from 'vitest';

it('exposes synchronous DOM matchers through the local Vitest augmentation', () => {
  const element = document.createElement('button');
  element.textContent = 'Save';

  expectTypeOf(expect(element).toHaveTextContent('Save')).toEqualTypeOf<void>();
  expect(element).toEqual(expect.toHaveTextContent('Save'));
});

it('preserves the promise return type of asynchronous DOM matchers', async () => {
  const element = document.createElement('button');
  element.textContent = 'Save';

  const assertion = expect(Promise.resolve(element)).resolves.toHaveTextContent('Save');
  expectTypeOf(assertion).toEqualTypeOf<Promise<void>>();
  await assertion;
});
