export async function fetchAll(ids, fetchOne) {
  const results = [];
  ids.forEach(async (id) => {
    const value = await fetchOne(id);
    results.push(value);
  });
  return results;
}
