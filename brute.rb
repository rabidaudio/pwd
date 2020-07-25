lower = 'a'..'z'
upper = 'A'..'Z'
numbers = '0'..'9'
symbols = ['.', '/', '-', '_', '!', '?', '@', '#', '$', '%', '^', '&', '*', '+', '=']
# obscure_symbols = ['\\', '(', ')', '{', '}', ]

def brute_force(path, space, sizes, processes)
  i = 0
  sizes.each do |size|
    space.permutation(size).map(&:join).each_slice(processes) do |guesses|
      procs = guesses.map do |password|
        spawn("7z x -y -p#{password} #{path}")
      end
        return password if try_lock(path, password)
        i += 1
        puts "#{i}: #{password}" if i % 10_000 == 0
      end
    end
  end
end

path = 'test.zip'
password = brute_force(path, lower.to_a, [6])

if password
  print("got em! #{password}")
else
  print("no luck")
end
