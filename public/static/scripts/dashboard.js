const tbody = document.getElementById("host-table")
const template = document.getElementById("host-row-template")

function toggleRow(button) {
      const tr = button.closest('tr');
      const nextRow = tr.nextElementSibling;
      const container = nextRow.querySelector('div');
      const arrow = button.querySelector('svg');
      const isCollapsed = container.classList.contains('max-h-0');

      if (isCollapsed) {
        container.classList.remove('max-h-0', 'opacity-0');
        container.classList.add('max-h-40', 'opacity-100');
        arrow.classList.add('rotate-180');
      } else {
        container.classList.add('max-h-0', 'opacity-0');
        container.classList.remove('max-h-40', 'opacity-100');
        arrow.classList.remove('rotate-180');
      }
}
window.toggleRow = toggleRow;

const users = [
    { id: 1, name: 'thingy', power: 'on', ram: '8' },
    { id: 2, name: 'other thingy', power: 'on', ram: '8' },
    { id: 3, name: 'long name thingy more words', power: 'off', ram: '16' },
    { id: 4, name: 'short', power: 'bruh', ram: '32' },
    { id: 5, name: 'another thingy', power: 'power stuff', ram: '8' },
    { id: 6, name: 'yet another thingy', power: 'on', ram: '4' },
    { id: 7, name: 'stuff', power: 'on', ram: '64' },
    { id: 8, name: 'more stuff', power: 'off', ram: '16' },
    { id: 9, name: 'things', power: 'on', ram: '2' },
    { id: 10, name: 'bruh', power: 'on', ram: '32' }
];


users.forEach(user => {
    const clone = template.content.cloneNode(true);
    clone.querySelector('[data-field="name"]').textContent = user.name;
    clone.querySelector('[data-field="power"]').textContent = user.power;
    clone.querySelector('[data-field="id"]').textContent = user.id;
    clone.querySelector('[data-field="ram"]').textContent = user.ram;
    tbody.appendChild(clone);
});
