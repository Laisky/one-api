/**
 * MESSAGE_ACTIONS_REVEAL_CLASS keeps per-message action menus usable without hover.
 * Touch devices (no hover) always show them; hover-capable devices reveal them on
 * message hover, on keyboard focus inside the message, and while their menu is open,
 * so moving the pointer into the portaled menu never hides the trigger.
 */
export const MESSAGE_ACTIONS_REVEAL_CLASS =
  'transition-opacity duration-200 no-touch:opacity-0 no-touch:group-hover:opacity-100 no-touch:group-focus-within:opacity-100 no-touch:has-[[data-state=open]]:opacity-100';
